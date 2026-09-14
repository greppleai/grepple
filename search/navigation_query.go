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
	// NavigationQueryDependencies follows outgoing calls from one or more scope roots.
	NavigationQueryDependencies NavigationQueryDirection = "dependencies"
	// NavigationQueryDependents follows incoming calls toward one or more scope roots.
	NavigationQueryDependents NavigationQueryDirection = "dependents"
	// NavigationQueryImpact traverses both incoming and outgoing calls.
	NavigationQueryImpact NavigationQueryDirection = "impact"
)

// QueryNavigationGraph returns the bounded directional subgraph reachable from
// root declaration IDs. Resolved and candidate targets are retained so callers
// can distinguish certain edges from ambiguous repository-local possibilities.
func QueryNavigationGraph(graph parser.NavigationGraph, rootIDs []string, direction NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	if !validNavigationQueryDirection(direction) {
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
	roots := copyQuerySet(frontier)
	visited := copyQuerySet(frontier)
	for level := 0; level < depth && len(frontier) > 0; level++ {
		frontier = queryNavigationLevel(graph.Calls, frontier, direction, level == 0, roots, visited, declarations, included, includedCalls)
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

func copyQuerySet(source map[string]bool) map[string]bool {
	copied := make(map[string]bool, len(source))
	for id := range source {
		copied[id] = true
	}
	return copied
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

func queryNavigationLevel(calls []parser.NavigationCall, frontier map[string]bool, direction NavigationQueryDirection, firstLevel bool, roots, visited map[string]bool, declarations map[string]parser.NavigationDeclaration, included, includedCalls map[string]bool) map[string]bool {
	next := map[string]bool{}
	for _, call := range calls {
		neighbors, matches := queryCallNeighbors(call, frontier, roots, direction, firstLevel)
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

func queryCallNeighbors(call parser.NavigationCall, frontier, roots map[string]bool, direction NavigationQueryDirection, firstLevel bool) ([]string, bool) {
	targets := queryCallTargets(call)
	outgoing := direction == NavigationQueryCallees || direction == NavigationQueryDependencies || direction == NavigationQueryImpact
	incoming := direction == NavigationQueryCallers || direction == NavigationQueryDependents || direction == NavigationQueryImpact
	neighbors := make([]string, 0, len(targets)+1)
	if outgoing && frontier[call.CallerID] {
		for _, target := range targets {
			if !firstLevel || direction == NavigationQueryCallees || !roots[target] {
				neighbors = append(neighbors, target)
			}
		}
	}
	if incoming && queryTargetsIntersect(targets, frontier) && (!firstLevel || direction == NavigationQueryCallers || !roots[call.CallerID]) {
		neighbors = append(neighbors, call.CallerID)
	}
	return neighbors, len(neighbors) > 0
}

func validNavigationQueryDirection(direction NavigationQueryDirection) bool {
	switch direction {
	case NavigationQueryCallers, NavigationQueryCallees, NavigationQueryDependencies, NavigationQueryDependents, NavigationQueryImpact:
		return true
	default:
		return false
	}
}

func queryTargetsIntersect(targets []string, frontier map[string]bool) bool {
	for _, id := range targets {
		if frontier[id] {
			return true
		}
	}
	return false
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
		Declarations:   make([]parser.NavigationDeclaration, 0, len(included)),
		Calls:          make([]parser.NavigationCall, 0, len(includedCalls)),
		Exports:        make([]parser.NavigationExport, 0),
		Fields:         make([]parser.NavigationField, 0),
		TypeUsages:     make([]parser.NavigationTypeUsage, 0),
		MemberAccesses: make([]parser.NavigationMemberAccess, 0),
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
	projectNavigationContextFacts(graph, &result)
	projectNavigationOwnedFacts(graph, included, &result)
	return result
}

func projectNavigationContextFacts(graph parser.NavigationGraph, result *parser.NavigationGraph) {
	fieldOwners := navigationQueryFieldOwners(result.Declarations, result.Calls)
	for _, field := range graph.Fields {
		if fieldOwners[terminalSymbolName(field.OwnerType)] {
			result.Fields = append(result.Fields, field)
		}
	}
	files := navigationQueryFiles(result.Declarations, result.Calls, result.Fields)
	for _, export := range graph.Exports {
		if files[export.Path] {
			result.Exports = append(result.Exports, export)
		}
	}
}

func projectNavigationOwnedFacts(graph parser.NavigationGraph, included map[string]bool, result *parser.NavigationGraph) {
	for _, usage := range graph.TypeUsages {
		if included[usage.CallerID] {
			result.TypeUsages = append(result.TypeUsages, usage)
		}
	}
	for _, access := range graph.MemberAccesses {
		if included[access.CallerID] {
			result.MemberAccesses = append(result.MemberAccesses, access)
		}
	}
}

func navigationQueryFieldOwners(declarations []parser.NavigationDeclaration, calls []parser.NavigationCall) map[string]bool {
	owners := make(map[string]bool)
	for _, declaration := range declarations {
		if declaration.Container != "" {
			owners[terminalSymbolName(declaration.Container)] = true
		}
		if declaration.Receiver != "" {
			owners[terminalSymbolName(declaration.Receiver)] = true
		}
	}
	for _, call := range calls {
		if call.ReceiverRootType != "" {
			owners[terminalSymbolName(call.ReceiverRootType)] = true
		}
		if call.ReceiverType != "" {
			owners[terminalSymbolName(call.ReceiverType)] = true
		}
	}
	return owners
}

func navigationQueryFiles(declarations []parser.NavigationDeclaration, calls []parser.NavigationCall, fields []parser.NavigationField) map[string]bool {
	files := make(map[string]bool)
	for _, declaration := range declarations {
		files[declaration.Path] = true
	}
	for _, call := range calls {
		files[call.Path] = true
	}
	for _, field := range fields {
		files[field.Path] = true
	}
	return files
}
