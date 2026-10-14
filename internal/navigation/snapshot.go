package navigation

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
)

// QuerySnapshot owns numeric adjacency and addressable native detail blocks,
// never the resolver, source text or a decoded graph. IDs are generation-local.
type QuerySnapshot interface {
	Query([]string, NavigationQueryDirection, int) (parser.NavigationGraph, error)
	Graph() (parser.NavigationGraph, error)
}

type snapshotCall struct {
	caller, targets, count   uint32
	direct, candidatesNonNil bool
}
type snapshotCSR struct{ offsets, edges []uint32 }
type querySnapshot struct {
	roots                   []string
	details                 *snapshotDetails
	ids                     map[string]uint32
	names                   []string
	declarations            map[uint32][]uint32
	calls                   []snapshotCall
	targets                 []uint32
	outgoing, incoming      snapshotCSR
	groups                  []uint32
	duplicates              map[uint32][]uint32
	types, imports, exports map[string][]uint32
	fields                  map[string][]uint32
	usages, accesses        map[uint32][]uint32
}

// newQuerySnapshot freezes query indexes without changing parser facts or
// resolutions. Callers must not mutate the input graph while it is referenced.
func newQuerySnapshot(graph parser.NavigationGraph) (QuerySnapshot, error) {
	capacity := uint64(len(graph.Declarations)) + uint64(len(graph.Calls))
	for _, call := range graph.Calls {
		capacity += uint64(len(queryCallTargets(call)))
	}
	if capacity >= math.MaxUint32 {
		return nil, fmt.Errorf("navigation snapshot exceeds uint32 capacity")
	}
	s := &querySnapshot{ids: make(map[string]uint32, len(graph.Declarations)), declarations: make(map[uint32][]uint32, len(graph.Declarations)), duplicates: map[uint32][]uint32{}, types: map[string][]uint32{}, imports: map[string][]uint32{}, exports: map[string][]uint32{}, fields: map[string][]uint32{}, usages: map[uint32][]uint32{}, accesses: map[uint32][]uint32{}}
	s.calls = make([]snapshotCall, 0, len(graph.Calls))
	s.groups = make([]uint32, 0, len(graph.Calls))
	s.targets = make([]uint32, 0, int(capacity)-len(graph.Declarations)-len(graph.Calls))
	for _, root := range graph.RepositoryRoots {
		s.roots = append(s.roots, strings.Clone(root))
	}
	intern := func(id string) uint32 {
		if node, ok := s.ids[id]; ok {
			return node
		}
		node := uint32(len(s.ids))
		name := strings.Clone(id)
		s.ids[name] = node
		s.names = append(s.names, name)
		return node
	}
	for row, d := range graph.Declarations {
		node := intern(d.ID)
		s.declarations[node] = append(s.declarations[node], uint32(row))
	}
	s.indexCalls(graph.Calls, intern)
	s.outgoing = s.buildCSR(false)
	s.incoming = s.buildCSR(true)
	s.indexContextFacts(graph)
	s.types = cloneSnapshotRowKeys(s.types)
	s.imports = cloneSnapshotRowKeys(s.imports)
	s.exports = cloneSnapshotRowKeys(s.exports)
	s.fields = cloneSnapshotRowKeys(s.fields)
	var err error
	s.details, err = newSnapshotDetails(graph)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Relation identities live once in numeric hot tables, not duplicated as huge
// per-call candidate string lists in the detail store. Hydration reconstructs
// exactly the original IDs and preserves candidates accompanying direct targets.
func (s *querySnapshot) readCalls(rows []uint32) ([]parser.NavigationCall, error) {
	calls, err := s.details.calls.read(rows)
	if err != nil {
		return nil, err
	}
	for i, row := range rows {
		record := s.calls[row]
		calls[i].CallerID = s.names[record.caller]
		if record.direct {
			calls[i].TargetID = s.names[s.targets[record.targets]]
			if record.candidatesNonNil && calls[i].CandidateTargetIDs == nil {
				calls[i].CandidateTargetIDs = []string{}
			}
			continue
		}
		if record.candidatesNonNil {
			calls[i].CandidateTargetIDs = make([]string, record.count)
			for n, node := range s.targets[record.targets : record.targets+record.count] {
				calls[i].CandidateTargetIDs[n] = s.names[node]
			}
		}
	}
	return calls, nil
}

func (s *querySnapshot) indexCalls(calls []parser.NavigationCall, intern func(string) uint32) {
	groups := map[string]uint32{}
	for row, c := range calls {
		record := snapshotCall{caller: intern(c.CallerID), targets: uint32(len(s.targets)), direct: c.TargetID != "", candidatesNonNil: c.CandidateTargetIDs != nil}
		for _, id := range queryCallTargets(c) {
			s.targets = append(s.targets, intern(id))
			record.count++
		}
		s.calls = append(s.calls, record)
		if first, ok := groups[c.ID]; ok {
			s.groups = append(s.groups, first)
			if len(s.duplicates[first]) == 0 {
				s.duplicates[first] = []uint32{first}
			}
			s.duplicates[first] = append(s.duplicates[first], uint32(row))
		} else {
			groups[c.ID] = uint32(row)
			s.groups = append(s.groups, uint32(row))
		}
	}
}

func (s *querySnapshot) indexContextFacts(graph parser.NavigationGraph) {
	for row, d := range graph.TypeDeclarations {
		s.types[d.Path] = append(s.types[d.Path], uint32(row))
	}
	for row, i := range graph.Imports {
		s.imports[i.Path] = append(s.imports[i.Path], uint32(row))
	}
	for row, e := range graph.Exports {
		s.exports[e.Path] = append(s.exports[e.Path], uint32(row))
	}
	for row, f := range graph.Fields {
		key := terminalSymbolName(f.OwnerType)
		s.fields[key] = append(s.fields[key], uint32(row))
	}
	for row, u := range graph.TypeUsages {
		if id, ok := s.ids[u.CallerID]; ok {
			s.usages[id] = append(s.usages[id], uint32(row))
		}
	}
	for row, a := range graph.MemberAccesses {
		if id, ok := s.ids[a.CallerID]; ok {
			s.accesses[id] = append(s.accesses[id], uint32(row))
		}
	}
}
func (s *querySnapshot) buildCSR(reverse bool) snapshotCSR {
	csr := snapshotCSR{offsets: make([]uint32, len(s.ids)+1)}
	each := func(c snapshotCall, visit func(uint32)) {
		if reverse {
			for _, node := range s.targets[c.targets : c.targets+c.count] {
				visit(node)
			}
		} else {
			visit(c.caller)
		}
	}
	for _, c := range s.calls {
		each(c, func(node uint32) { csr.offsets[node+1]++ })
	}
	for n := 1; n < len(csr.offsets); n++ {
		csr.offsets[n] += csr.offsets[n-1]
	}
	csr.edges = make([]uint32, csr.offsets[len(csr.offsets)-1])
	positions := append([]uint32(nil), csr.offsets...)
	for row, c := range s.calls {
		each(c, func(node uint32) { csr.edges[positions[node]] = uint32(row); positions[node]++ })
	}
	return csr
}

type snapshotQueryState struct {
	direction                                    NavigationQueryDirection
	roots, frontier, included, visited, selected map[uint32]bool
}

func (s *querySnapshot) Query(rootIDs []string, direction NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	if !validNavigationQueryDirection(direction) {
		return parser.NavigationGraph{}, fmt.Errorf("unsupported navigation query direction %q", direction)
	}
	if depth < 1 {
		return parser.NavigationGraph{}, fmt.Errorf("navigation query depth must be positive")
	}
	state, err := s.queryRoots(rootIDs, direction)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	for level := 0; level < depth && len(state.frontier) > 0; level++ {
		state.frontier = s.queryLevel(state, level == 0)
	}
	return s.project(state.included, state.selected)
}

func (s *querySnapshot) queryRoots(rootIDs []string, direction NavigationQueryDirection) (*snapshotQueryState, error) {
	if len(rootIDs) == 0 {
		return nil, fmt.Errorf("navigation query requires at least one root declaration")
	}
	state := &snapshotQueryState{direction: direction, roots: map[uint32]bool{}, frontier: map[uint32]bool{}, included: map[uint32]bool{}, visited: map[uint32]bool{}, selected: map[uint32]bool{}}
	for _, id := range rootIDs {
		node, ok := s.ids[id]
		if !ok || len(s.declarations[node]) == 0 {
			return nil, fmt.Errorf("navigation query root %q does not exist", id)
		}
		state.roots[node] = true
		state.frontier[node] = true
		state.included[node] = true
		state.visited[node] = true
	}
	return state, nil
}

func (s *querySnapshot) queryEdges(state *snapshotQueryState) map[uint32]bool {
	touched := map[uint32]bool{}
	outgoing := state.direction == NavigationQueryCallees || state.direction == NavigationQueryDependencies || state.direction == NavigationQueryImpact
	incoming := state.direction == NavigationQueryCallers || state.direction == NavigationQueryDependents || state.direction == NavigationQueryImpact
	for node := range state.frontier {
		if outgoing {
			addSnapshotRows(touched, s.outgoing.edges[s.outgoing.offsets[node]:s.outgoing.offsets[node+1]])
		}
		if incoming {
			addSnapshotRows(touched, s.incoming.edges[s.incoming.offsets[node]:s.incoming.offsets[node+1]])
		}
	}
	return touched
}

func (s *querySnapshot) queryLevel(state *snapshotQueryState, first bool) map[uint32]bool {
	next := map[uint32]bool{}
	for edge := range s.queryEdges(state) {
		call := s.calls[edge]
		targets := s.targets[call.targets : call.targets+call.count]
		neighbors := snapshotOutgoingNeighbors(call.caller, targets, state, first)
		neighbors = append(neighbors, snapshotIncomingNeighbors(call.caller, targets, state, first)...)
		if len(neighbors) == 0 {
			continue
		}
		group := s.groups[edge]
		state.selected[group] = true
		addSnapshotRows(state.selected, s.duplicates[group])
		for _, node := range neighbors {
			if len(s.declarations[node]) > 0 && !state.visited[node] {
				state.visited[node] = true
				next[node] = true
			}
		}
		for _, node := range append([]uint32{call.caller}, targets...) {
			if len(s.declarations[node]) > 0 {
				state.included[node] = true
			}
		}
	}
	return next
}

func snapshotOutgoingNeighbors(caller uint32, targets []uint32, state *snapshotQueryState, first bool) []uint32 {
	outgoing := state.direction == NavigationQueryCallees || state.direction == NavigationQueryDependencies || state.direction == NavigationQueryImpact
	if !outgoing || !state.frontier[caller] {
		return nil
	}
	neighbors := []uint32{}
	for _, target := range targets {
		if !first || state.direction == NavigationQueryCallees || !state.roots[target] {
			neighbors = append(neighbors, target)
		}
	}
	return neighbors
}

func snapshotIncomingNeighbors(caller uint32, targets []uint32, state *snapshotQueryState, first bool) []uint32 {
	incoming := state.direction == NavigationQueryCallers || state.direction == NavigationQueryDependents || state.direction == NavigationQueryImpact
	if !incoming {
		return nil
	}
	if first && state.direction != NavigationQueryCallers && state.roots[caller] {
		return nil
	}
	for _, target := range targets {
		if state.frontier[target] {
			return []uint32{caller}
		}
	}
	return nil
}
func sortedSnapshotRows(set map[uint32]bool) []uint32 {
	rows := make([]uint32, 0, len(set))
	for row := range set {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i] < rows[j] })
	return rows
}
func addSnapshotRows(set map[uint32]bool, rows []uint32) {
	for _, row := range rows {
		set[row] = true
	}
}

func (s *querySnapshot) project(included, selected map[uint32]bool) (parser.NavigationGraph, error) {
	result := parser.NavigationGraph{RepositoryRoots: append([]string(nil), s.roots...)}
	var err error
	rows := map[uint32]bool{}
	usageRows, accessRows := map[uint32]bool{}, map[uint32]bool{}
	for node := range included {
		addSnapshotRows(rows, s.declarations[node])
		addSnapshotRows(usageRows, s.usages[node])
		addSnapshotRows(accessRows, s.accesses[node])
	}
	result.Declarations, err = s.details.declarations.read(sortedSnapshotRows(rows))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.Calls, err = s.readCalls(sortedSnapshotRows(selected))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	rows = map[uint32]bool{}
	for owner := range navigationQueryFieldOwners(result.Declarations, result.Calls) {
		addSnapshotRows(rows, s.fields[owner])
	}
	result.Fields, err = s.details.fields.read(sortedSnapshotRows(rows))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	typeRows, importRows, exportRows := map[uint32]bool{}, map[uint32]bool{}, map[uint32]bool{}
	for file := range navigationQueryFiles(result.Declarations, result.Calls, result.Fields) {
		addSnapshotRows(typeRows, s.types[file])
		addSnapshotRows(importRows, s.imports[file])
		addSnapshotRows(exportRows, s.exports[file])
	}
	result.TypeDeclarations, err = s.details.types.read(sortedSnapshotRows(typeRows))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.Imports, err = s.details.imports.read(sortedSnapshotRows(importRows))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.Exports, err = s.details.exports.read(sortedSnapshotRows(exportRows))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.TypeUsages, err = s.details.usages.read(sortedSnapshotRows(usageRows))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.MemberAccesses, err = s.details.accesses.read(sortedSnapshotRows(accessRows))
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	return result, nil
}
func allSnapshotRows(count uint32) []uint32 {
	rows := make([]uint32, count)
	for i := range rows {
		rows[i] = uint32(i)
	}
	return rows
}
func (s *querySnapshot) Graph() (parser.NavigationGraph, error) {
	result := parser.NavigationGraph{RepositoryRoots: append([]string(nil), s.roots...)}
	var err error
	result.Declarations, err = s.details.declarations.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	if s.details.calls.nonNil {
		result.Calls, err = s.readCalls(allSnapshotRows(s.details.calls.count))
	}
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.TypeDeclarations, err = s.details.types.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.Imports, err = s.details.imports.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.Exports, err = s.details.exports.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.Fields, err = s.details.fields.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.TypeUsages, err = s.details.usages.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	result.MemberAccesses, err = s.details.accesses.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	return result, nil
}
