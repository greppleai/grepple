package search

import (
	"fmt"

	"github.com/greppleai/grepple/parser"
)

// NavigationQueryDirection selects incoming or outgoing repository-local calls.
type NavigationQueryDirection string

const (
	// NavigationQueryCallers follows calls toward declarations that may call the roots.
	NavigationQueryCallers NavigationQueryDirection = "callers"
	// NavigationQueryCallees follows calls toward declarations called by the roots.
	NavigationQueryCallees NavigationQueryDirection = "callees"
)

// QueryNavigationGraph returns the bounded directional subgraph reachable from
// root declaration IDs. Resolved and candidate targets are retained so callers
// can distinguish certain edges from ambiguous repository-local possibilities.
func QueryNavigationGraph(graph parser.NavigationGraph, rootIDs []string, direction NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	if direction != NavigationQueryCallers && direction != NavigationQueryCallees {
		return parser.NavigationGraph{}, fmt.Errorf("unsupported navigation query direction %q", direction)
	}
	if depth < 1 {
		return parser.NavigationGraph{}, fmt.Errorf("navigation query depth must be positive")
	}
	declarations := indexQueryDeclarations(graph.Declarations)
	frontier, included, err := queryRoots(rootIDs, declarations)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	includedCalls := map[string]bool{}
	visited := make(map[string]bool, len(frontier))
	for id := range frontier {
		visited[id] = true
	}
	for level := 0; level < depth && len(frontier) > 0; level++ {
		frontier = queryNavigationLevel(graph.Calls, frontier, direction, declarations, visited, included, includedCalls)
	}
	return projectNavigationQuery(graph, included, includedCalls), nil
}

func indexQueryDeclarations(declarations []parser.NavigationDeclaration) map[string]parser.NavigationDeclaration {
	indexed := make(map[string]parser.NavigationDeclaration, len(declarations))
	for _, declaration := range declarations {
		indexed[declaration.ID] = declaration
	}
	return indexed
}

func queryRoots(rootIDs []string, declarations map[string]parser.NavigationDeclaration) (map[string]bool, map[string]bool, error) {
	if len(rootIDs) == 0 {
		return nil, nil, fmt.Errorf("navigation query requires at least one root declaration")
	}
	frontier := make(map[string]bool, len(rootIDs))
	included := make(map[string]bool, len(rootIDs))
	for _, id := range rootIDs {
		if _, ok := declarations[id]; !ok {
			return nil, nil, fmt.Errorf("navigation query root %q does not exist", id)
		}
		frontier[id] = true
		included[id] = true
	}
	return frontier, included, nil
}

func queryNavigationLevel(calls []parser.NavigationCall, frontier map[string]bool, direction NavigationQueryDirection, declarations map[string]parser.NavigationDeclaration, visited, included, includedCalls map[string]bool) map[string]bool {
	next := map[string]bool{}
	for _, call := range calls {
		neighbors, matches := queryCallNeighbors(call, frontier, direction)
		if !matches {
			continue
		}
		includedCalls[call.ID] = true
		for _, id := range neighbors {
			if _, ok := declarations[id]; ok && !visited[id] {
				visited[id] = true
				next[id] = true
			}
		}
		includeQueryCallDeclarations(call, declarations, included)
	}
	return next
}

func queryCallNeighbors(call parser.NavigationCall, frontier map[string]bool, direction NavigationQueryDirection) ([]string, bool) {
	if direction == NavigationQueryCallees {
		if !frontier[call.CallerID] {
			return nil, false
		}
		targets := queryCallTargets(call)
		return targets, len(targets) > 0
	}
	for _, id := range queryCallTargets(call) {
		if frontier[id] {
			return []string{call.CallerID}, true
		}
	}
	return nil, false
}

func queryCallTargets(call parser.NavigationCall) []string {
	if call.TargetID != "" {
		return []string{call.TargetID}
	}
	return call.CandidateTargetIDs
}

func includeQueryCallDeclarations(call parser.NavigationCall, declarations map[string]parser.NavigationDeclaration, included map[string]bool) {
	if _, ok := declarations[call.CallerID]; ok {
		included[call.CallerID] = true
	}
	for _, id := range queryCallTargets(call) {
		if _, ok := declarations[id]; ok {
			included[id] = true
		}
	}
}

func projectNavigationQuery(graph parser.NavigationGraph, included, includedCalls map[string]bool) parser.NavigationGraph {
	result := parser.NavigationGraph{
		Declarations: make([]parser.NavigationDeclaration, 0, len(included)),
		Calls:        make([]parser.NavigationCall, 0, len(includedCalls)),
	}
	for _, declaration := range graph.Declarations {
		if included[declaration.ID] {
			result.Declarations = append(result.Declarations, declaration)
		}
	}
	for _, call := range graph.Calls {
		if includedCalls[call.ID] {
			result.Calls = append(result.Calls, call)
		}
	}
	return result
}
