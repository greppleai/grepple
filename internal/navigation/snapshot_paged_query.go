package navigation

import (
	"fmt"
	"github.com/greppleai/grepple/internal/parser"
	"sort"
)

func queryPagedSnapshot(s *pagedStoredSnapshot, ids []string, d NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	if !validNavigationQueryDirection(d) || depth < 1 || len(ids) == 0 {
		return parser.NavigationGraph{}, fmt.Errorf("invalid paged navigation query")
	}
	r := newPagedQueryReader(s)
	state := &snapshotQueryState{direction: d, roots: map[uint32]bool{}, frontier: map[uint32]bool{}, included: map[uint32]bool{}, visited: map[uint32]bool{}, selected: map[uint32]bool{}}
	for _, id := range ids {
		nodes := r.keyRows("lookup", id)
		if r.err != nil {
			return parser.NavigationGraph{}, r.err
		}
		if len(nodes) != 1 {
			return parser.NavigationGraph{}, fmt.Errorf("navigation query root %q does not exist", id)
		}
		node := nodes[0]
		declarations := r.rows("decl", node)
		if r.err != nil {
			return parser.NavigationGraph{}, r.err
		}
		if len(declarations) == 0 {
			return parser.NavigationGraph{}, fmt.Errorf("navigation query root %q does not exist", id)
		}
		state.roots[node] = true
		state.frontier[node] = true
		state.included[node] = true
		state.visited[node] = true
	}
	for level := 0; level < depth && len(state.frontier) > 0 && r.err == nil; level++ {
		pagedQueryLevel(r, state, level == 0)
	}
	if r.err != nil {
		return parser.NavigationGraph{}, r.err
	}
	return projectPagedSnapshot(r, state.included, state.selected)
}
func pagedQueryLevel(r *pagedQueryReader, state *snapshotQueryState, first bool) {
	touched := map[uint32]bool{}
	for _, node := range sortedSnapshotRows(state.frontier) {
		if state.direction == NavigationQueryCallees || state.direction == NavigationQueryDependencies || state.direction == NavigationQueryImpact {
			addSnapshotRows(touched, r.rows("out", node))
		}
		if state.direction == NavigationQueryCallers || state.direction == NavigationQueryDependents || state.direction == NavigationQueryImpact {
			addSnapshotRows(touched, r.rows("in", node))
		}
	}
	next := map[uint32]bool{}
	for _, edge := range sortedSnapshotRows(touched) {
		pagedQueryEdge(r, state, first, edge, next)
		if r.err != nil {
			return
		}
	}
	state.frontier = next
	if len(state.visited)+len(state.selected) > pagedSelectionLimit {
		r.err = fmt.Errorf("%w: paged traversal selection limit", ErrSnapshotBudget)
	}
}
func pagedQueryEdge(r *pagedQueryReader, state *snapshotQueryState, first bool, edge uint32, next map[uint32]bool) {
	call, targets := r.call(edge)
	neighbors := snapshotOutgoingNeighbors(call.caller, targets, state, first)
	neighbors = append(neighbors, snapshotIncomingNeighbors(call.caller, targets, state, first)...)
	if r.err != nil || len(neighbors) == 0 {
		return
	}
	group := r.number("groups", edge)
	state.selected[group] = true
	addSnapshotRows(state.selected, r.rows("dup", group))
	for _, node := range neighbors {
		if len(r.rows("decl", node)) > 0 && !state.visited[node] {
			state.visited[node] = true
			next[node] = true
		}
	}
	for _, node := range append([]uint32{call.caller}, targets...) {
		if len(r.rows("decl", node)) > 0 {
			state.included[node] = true
		}
	}
}
func readPagedCalls(r *pagedQueryReader, rows []uint32) ([]parser.NavigationCall, error) {
	calls, err := r.s.details.calls.read(rows)
	if err != nil {
		return nil, err
	}
	for i, row := range rows {
		call, targets := r.call(row)
		if r.err != nil {
			return nil, r.err
		}
		calls[i].CallerID = r.name(call.caller)
		if call.direct {
			calls[i].TargetID = r.name(targets[0])
			if call.candidatesNonNil && calls[i].CandidateTargetIDs == nil {
				calls[i].CandidateTargetIDs = []string{}
			}
		} else if call.candidatesNonNil {
			calls[i].CandidateTargetIDs = make([]string, len(targets))
			for j, node := range targets {
				calls[i].CandidateTargetIDs[j] = r.name(node)
			}
		}
	}
	return calls, r.err
}
func projectPagedSnapshot(r *pagedQueryReader, included, selected map[uint32]bool) (parser.NavigationGraph, error) {
	result := parser.NavigationGraph{RepositoryRoots: append([]string(nil), r.s.manifest.Roots...)}
	rows, usages, accesses := map[uint32]bool{}, map[uint32]bool{}, map[uint32]bool{}
	for _, node := range sortedSnapshotRows(included) {
		addSnapshotRows(rows, r.rows("decl", node))
		addSnapshotRows(usages, r.rows("usage", node))
		addSnapshotRows(accesses, r.rows("access", node))
	}
	if r.err != nil {
		return result, r.err
	}
	var err error
	result.Declarations, err = r.s.details.declarations.read(sortedSnapshotRows(rows))
	if err != nil {
		return result, err
	}
	result.Calls, err = readPagedCalls(r, sortedSnapshotRows(selected))
	if err != nil {
		return result, err
	}
	rows = map[uint32]bool{}
	for _, owner := range sortedPagedKeys(navigationQueryFieldOwners(result.Declarations, result.Calls)) {
		addSnapshotRows(rows, r.keyRows("fields", owner))
	}
	if r.err != nil {
		return result, r.err
	}
	result.Fields, err = r.s.details.fields.read(sortedSnapshotRows(rows))
	if err != nil {
		return result, err
	}
	result, err = projectPagedFiles(r, result)
	if err != nil {
		return result, err
	}
	result.TypeUsages, err = r.s.details.usages.read(sortedSnapshotRows(usages))
	if err != nil {
		return result, err
	}
	result.MemberAccesses, err = r.s.details.accesses.read(sortedSnapshotRows(accesses))
	return result, err
}
func projectPagedFiles(r *pagedQueryReader, result parser.NavigationGraph) (parser.NavigationGraph, error) {
	types, imports, exports := map[uint32]bool{}, map[uint32]bool{}, map[uint32]bool{}
	for _, path := range sortedPagedKeys(navigationQueryFiles(result.Declarations, result.Calls, result.Fields)) {
		addSnapshotRows(types, r.keyRows("types", path))
		addSnapshotRows(imports, r.keyRows("imports", path))
		addSnapshotRows(exports, r.keyRows("exports", path))
	}
	if r.err != nil {
		return result, r.err
	}
	var err error
	result.TypeDeclarations, err = r.s.details.types.read(sortedSnapshotRows(types))
	if err != nil {
		return result, err
	}
	result.Imports, err = r.s.details.imports.read(sortedSnapshotRows(imports))
	if err != nil {
		return result, err
	}
	result.Exports, err = r.s.details.exports.read(sortedSnapshotRows(exports))
	return result, err
}
func graphPagedSnapshot(s *pagedStoredSnapshot) (parser.NavigationGraph, error) {
	r := newPagedQueryReader(s)
	result := parser.NavigationGraph{RepositoryRoots: append([]string(nil), s.manifest.Roots...)}
	var err error
	result.Declarations, err = s.details.declarations.readAll()
	if err != nil {
		return result, err
	}
	if s.details.calls.nonNil {
		result.Calls, err = readPagedCalls(r, allSnapshotRows(s.details.calls.count))
		if err != nil {
			return result, err
		}
	}
	result.TypeDeclarations, err = s.details.types.readAll()
	if err != nil {
		return result, err
	}
	result.Imports, err = s.details.imports.readAll()
	if err != nil {
		return result, err
	}
	result.Exports, err = s.details.exports.readAll()
	if err != nil {
		return result, err
	}
	result.Fields, err = s.details.fields.readAll()
	if err != nil {
		return result, err
	}
	result.TypeUsages, err = s.details.usages.readAll()
	if err != nil {
		return result, err
	}
	result.MemberAccesses, err = s.details.accesses.readAll()
	return result, err
}
func sortedPagedKeys(keys map[string]bool) []string {
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
