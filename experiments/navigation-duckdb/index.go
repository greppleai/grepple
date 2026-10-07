package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

type request struct {
	Roots     []string                            `json:"roots"`
	Direction navigation.NavigationQueryDirection `json:"direction"`
	Depth     int                                 `json:"depth"`
}
type queryIndex interface {
	Query(context.Context, request) (parser.NavigationGraph, error)
	Close() error
}
type rowCall struct {
	Ord  int
	Call parser.NavigationCall
}
type fragmentStore interface {
	Existing(context.Context, []string) ([]string, error)
	Calls(context.Context, []string, navigation.NavigationQueryDirection) ([]rowCall, error)
	Fragment(context.Context, []string, []int) (parser.NavigationGraph, error)
	Close() error
}

type scanIndex struct{ graph parser.NavigationGraph }

func (index *scanIndex) Query(_ context.Context, q request) (parser.NavigationGraph, error) {
	return navigation.NewGraphOperations().Query(index.graph, q.Roots, q.Direction, q.Depth)
}
func (*scanIndex) Close() error { return nil }

type indexedQuery struct{ store fragmentStore }

func (index *indexedQuery) Close() error { return index.store.Close() }
func (index *indexedQuery) Query(ctx context.Context, q request) (parser.NavigationGraph, error) {
	if q.Depth < 1 {
		return parser.NavigationGraph{}, fmt.Errorf("navigation query depth must be positive")
	}
	frontier, visited, included, roots := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, id := range q.Roots {
		frontier[id] = true
		visited[id] = true
		included[id] = true
		roots[id] = true
	}
	callRows := map[int]bool{}
	for level := 0; level < q.Depth && len(frontier) > 0; level++ {
		calls, err := index.store.Calls(ctx, keys(frontier), q.Direction)
		if err != nil {
			return parser.NavigationGraph{}, err
		}
		next := map[string]bool{}
		for _, row := range calls {
			call := row.Call
			targets := callTargets(call)
			neighbors := []string{}
			outgoing := q.Direction == navigation.NavigationQueryCallees || q.Direction == navigation.NavigationQueryDependencies || q.Direction == navigation.NavigationQueryImpact
			incoming := q.Direction == navigation.NavigationQueryCallers || q.Direction == navigation.NavigationQueryDependents || q.Direction == navigation.NavigationQueryImpact
			if outgoing && frontier[call.CallerID] {
				for _, target := range targets {
					if level != 0 || q.Direction == navigation.NavigationQueryCallees || !roots[target] {
						neighbors = append(neighbors, target)
					}
				}
			}
			if incoming && intersects(targets, frontier) && (level != 0 || q.Direction == navigation.NavigationQueryCallers || !roots[call.CallerID]) {
				neighbors = append(neighbors, call.CallerID)
			}
			if len(neighbors) == 0 {
				continue
			}
			callRows[row.Ord] = true
			included[call.CallerID] = true
			for _, id := range targets {
				included[id] = true
			}
			for _, id := range neighbors {
				if !visited[id] {
					visited[id] = true
					next[id] = true
				}
			}
		}
		if level+1 < q.Depth && len(next) > 0 {
			valid, err := index.store.Existing(ctx, keys(next))
			if err != nil {
				return parser.NavigationGraph{}, err
			}
			next = map[string]bool{}
			for _, id := range valid {
				next[id] = true
			}
		}
		frontier = next
	}
	ords := make([]int, 0, len(callRows))
	for ord := range callRows {
		ords = append(ords, ord)
	}
	sort.Ints(ords)
	graph, err := index.store.Fragment(ctx, keys(included), ords)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	// Production's own bounded projection is the semantic oracle, including context
	// facts, ambiguous candidates, cyclic traversal and first-level scope rules.
	return navigation.NewGraphOperations().Query(graph, q.Roots, q.Direction, q.Depth)
}
func callTargets(call parser.NavigationCall) []string {
	if call.TargetID != "" {
		return []string{call.TargetID}
	}
	return call.CandidateTargetIDs
}
func intersects(ids []string, set map[string]bool) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}
func keys(set map[string]bool) []string {
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
func terminal(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == ':' || r == '#' })
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
func emptyGraph() parser.NavigationGraph {
	return parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{}, Calls: []parser.NavigationCall{}, TypeDeclarations: []parser.NavigationTypeDeclaration{}, Imports: []parser.NavigationImport{}, Exports: []parser.NavigationExport{}, Fields: []parser.NavigationField{}, TypeUsages: []parser.NavigationTypeUsage{}, MemberAccesses: []parser.NavigationMemberAccess{}}
}
func contextKeys(graph parser.NavigationGraph) (map[string]bool, map[string]bool) {
	owners, files := map[string]bool{}, map[string]bool{}
	for _, d := range graph.Declarations {
		files[d.Path] = true
		if d.Container != "" {
			owners[terminal(d.Container)] = true
		}
		if d.Receiver != "" {
			owners[terminal(d.Receiver)] = true
		}
	}
	for _, c := range graph.Calls {
		files[c.Path] = true
		if c.ReceiverRootType != "" {
			owners[terminal(c.ReceiverRootType)] = true
		}
		if c.ReceiverType != "" {
			owners[terminal(c.ReceiverType)] = true
		}
	}
	return owners, files
}

type adjacencyIndex struct {
	graph                   parser.NavigationGraph
	defs                    map[string][]int
	outgoing, incoming      map[string][]int
	fields                  map[string][]int
	types, imports, exports map[string][]int
	usages, members         map[string][]int
}

func newAdjacency(graph parser.NavigationGraph) *indexedQuery {
	a := &adjacencyIndex{graph: graph, defs: map[string][]int{}, outgoing: map[string][]int{}, incoming: map[string][]int{}, fields: map[string][]int{}, types: map[string][]int{}, imports: map[string][]int{}, exports: map[string][]int{}, usages: map[string][]int{}, members: map[string][]int{}}
	for i, d := range graph.Declarations {
		a.defs[d.ID] = append(a.defs[d.ID], i)
	}
	for i, c := range graph.Calls {
		a.outgoing[c.CallerID] = append(a.outgoing[c.CallerID], i)
		for _, target := range callTargets(c) {
			a.incoming[target] = append(a.incoming[target], i)
		}
	}
	for i, f := range graph.Fields {
		a.fields[terminal(f.OwnerType)] = append(a.fields[terminal(f.OwnerType)], i)
	}
	for i, f := range graph.TypeDeclarations {
		a.types[f.Path] = append(a.types[f.Path], i)
	}
	for i, f := range graph.Imports {
		a.imports[f.Path] = append(a.imports[f.Path], i)
	}
	for i, f := range graph.Exports {
		a.exports[f.Path] = append(a.exports[f.Path], i)
	}
	for i, f := range graph.TypeUsages {
		a.usages[f.CallerID] = append(a.usages[f.CallerID], i)
	}
	for i, f := range graph.MemberAccesses {
		a.members[f.CallerID] = append(a.members[f.CallerID], i)
	}
	return &indexedQuery{store: a}
}
func (*adjacencyIndex) Close() error { return nil }
func (a *adjacencyIndex) Calls(_ context.Context, frontier []string, dir navigation.NavigationQueryDirection) ([]rowCall, error) {
	selected := map[int]bool{}
	for _, id := range frontier {
		if dir != navigation.NavigationQueryCallers && dir != navigation.NavigationQueryDependents {
			for _, ord := range a.outgoing[id] {
				selected[ord] = true
			}
		}
		if dir != navigation.NavigationQueryCallees && dir != navigation.NavigationQueryDependencies {
			for _, ord := range a.incoming[id] {
				selected[ord] = true
			}
		}
	}
	ords := sortedOrdinals(selected)
	rows := make([]rowCall, 0, len(ords))
	for _, ord := range ords {
		rows = append(rows, rowCall{ord, a.graph.Calls[ord]})
	}
	return rows, nil
}
func sortedOrdinals(set map[int]bool) []int {
	result := make([]int, 0, len(set))
	for n := range set {
		result = append(result, n)
	}
	sort.Ints(result)
	return result
}
func collectOrdinals(index map[string][]int, selected []string) []int {
	set := map[int]bool{}
	for _, key := range selected {
		for _, ord := range index[key] {
			set[ord] = true
		}
	}
	return sortedOrdinals(set)
}
func (a *adjacencyIndex) Fragment(_ context.Context, ids []string, ords []int) (parser.NavigationGraph, error) {
	g := emptyGraph()
	g.RepositoryRoots = a.graph.RepositoryRoots
	for _, i := range collectOrdinals(a.defs, ids) {
		g.Declarations = append(g.Declarations, a.graph.Declarations[i])
	}
	for _, i := range ords {
		g.Calls = append(g.Calls, a.graph.Calls[i])
	}
	owners, files := contextKeys(g)
	for _, i := range collectOrdinals(a.fields, keys(owners)) {
		f := a.graph.Fields[i]
		g.Fields = append(g.Fields, f)
		files[f.Path] = true
	}
	paths := keys(files)
	for _, i := range collectOrdinals(a.types, paths) {
		g.TypeDeclarations = append(g.TypeDeclarations, a.graph.TypeDeclarations[i])
	}
	for _, i := range collectOrdinals(a.imports, paths) {
		g.Imports = append(g.Imports, a.graph.Imports[i])
	}
	for _, i := range collectOrdinals(a.exports, paths) {
		g.Exports = append(g.Exports, a.graph.Exports[i])
	}
	for _, i := range collectOrdinals(a.usages, ids) {
		g.TypeUsages = append(g.TypeUsages, a.graph.TypeUsages[i])
	}
	for _, i := range collectOrdinals(a.members, ids) {
		g.MemberAccesses = append(g.MemberAccesses, a.graph.MemberAccesses[i])
	}
	return g, nil
}

func (a *adjacencyIndex) Existing(_ context.Context, ids []string) ([]string, error) {
	result := []string{}
	for _, id := range ids {
		if len(a.defs[id]) > 0 {
			result = append(result, id)
		}
	}
	return result, nil
}
