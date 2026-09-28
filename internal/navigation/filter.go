package navigation

import (
	"fmt"
	"sort"

	"github.com/greppleai/grepple/internal/parser"
)

// NavigationGraphFilter restricts declarations and calls before graph traversal.
// Empty fields retain every value. Values are exact canonical IDs.
type NavigationGraphFilter struct {
	Languages    []string
	Confidences  []string
	Visibilities []string
}

// filterNavigationGraph applies deterministic language and confidence filters
// while preserving declaration/call order and referential integrity.
func filterNavigationGraph(graph parser.NavigationGraph, filter NavigationGraphFilter) (parser.NavigationGraph, error) {
	languages, err := navigationLanguageFilter(filter.Languages)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	confidences, err := navigationConfidenceFilter(filter.Confidences)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	visibilities, err := navigationVisibilityFilter(filter.Visibilities)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	declarations, included := filterNavigationDeclarations(graph.Declarations, languages, visibilities)
	typeDeclarations := filterNavigationTypeDeclarations(graph.TypeDeclarations, languages)
	imports := filterNavigationImports(graph.Imports, languages)
	calls := filterNavigationCalls(graph.Calls, included, languages, confidences)
	exports := filterNavigationExports(graph.Exports, languages)
	fields := filterNavigationFields(graph.Fields, languages)
	usages := filterNavigationTypeUsages(graph.TypeUsages, included, languages)
	accesses := filterNavigationMemberAccesses(graph.MemberAccesses, included, languages)
	return parser.NavigationGraph{Declarations: declarations, TypeDeclarations: typeDeclarations, Calls: calls, Imports: imports, Exports: exports, Fields: fields, TypeUsages: usages, MemberAccesses: accesses, RepositoryRoots: append([]string(nil), graph.RepositoryRoots...)}, nil
}

// normalizeNavigationGraphFilter returns sorted, duplicate-free values after validation.
func normalizeNavigationGraphFilter(filter NavigationGraphFilter) (NavigationGraphFilter, error) {
	languages, err := navigationLanguageFilter(filter.Languages)
	if err != nil {
		return NavigationGraphFilter{}, err
	}
	confidences, err := navigationConfidenceFilter(filter.Confidences)
	if err != nil {
		return NavigationGraphFilter{}, err
	}
	visibilities, err := navigationVisibilityFilter(filter.Visibilities)
	if err != nil {
		return NavigationGraphFilter{}, err
	}
	return NavigationGraphFilter{Languages: sortedFilterKeys(languages), Confidences: sortedFilterKeys(confidences), Visibilities: sortedFilterKeys(visibilities)}, nil
}

func navigationLanguageFilter(values []string) (map[string]bool, error) {
	filter := make(map[string]bool, len(values))
	for _, value := range values {
		capabilities, ok := parser.NewParser().CapabilitiesForLanguage(value)
		if !ok || !capabilities.Navigation {
			return nil, fmt.Errorf("unsupported navigation language %q", value)
		}
		filter[value] = true
	}
	return filter, nil
}

func navigationConfidenceFilter(values []string) (map[string]bool, error) {
	filter := make(map[string]bool, len(values))
	for _, value := range values {
		switch value {
		case "exact", "import-resolved", "context-resolved", "unique-terminal", "candidate":
			filter[value] = true
		default:
			return nil, fmt.Errorf("unsupported navigation confidence %q", value)
		}
	}
	return filter, nil
}

func navigationVisibilityFilter(values []string) (map[string]bool, error) {
	filter := make(map[string]bool, len(values))
	for _, value := range values {
		switch parser.NavigationVisibility(value) {
		case parser.NavigationVisibilityPublic, parser.NavigationVisibilityNonPublic, parser.NavigationVisibilityUnknown:
			filter[value] = true
		default:
			return nil, fmt.Errorf("unsupported navigation visibility %q", value)
		}
	}
	return filter, nil
}

func filterNavigationDeclarations(declarations []parser.NavigationDeclaration, languages, visibilities map[string]bool) ([]parser.NavigationDeclaration, map[string]bool) {
	filtered := make([]parser.NavigationDeclaration, 0, len(declarations))
	included := make(map[string]bool, len(declarations))
	for _, declaration := range declarations {
		if len(languages) > 0 && !languages[declaration.Language] || len(visibilities) > 0 && !visibilities[string(declaration.Visibility)] {
			continue
		}
		filtered = append(filtered, declaration)
		included[declaration.ID] = true
	}
	return filtered, included
}

func filterNavigationCalls(calls []parser.NavigationCall, declarations, languages, confidences map[string]bool) []parser.NavigationCall {
	filtered := make([]parser.NavigationCall, 0, len(calls))
	for _, call := range calls {
		if !declarations[call.CallerID] || len(languages) > 0 && !languages[call.Language] || len(confidences) > 0 && !confidences[call.Confidence] {
			continue
		}
		if call.TargetID != "" && !declarations[call.TargetID] {
			continue
		}
		call.CandidateTargetIDs = filterNavigationTargetIDs(call.CandidateTargetIDs, declarations)
		filtered = append(filtered, call)
	}
	return filtered
}

func filterNavigationTypeDeclarations(declarations []parser.NavigationTypeDeclaration, languages map[string]bool) []parser.NavigationTypeDeclaration {
	filtered := make([]parser.NavigationTypeDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		if len(languages) == 0 || languages[declaration.Language] {
			filtered = append(filtered, declaration)
		}
	}
	return filtered
}

func filterNavigationImports(imports []parser.NavigationImport, languages map[string]bool) []parser.NavigationImport {
	filtered := make([]parser.NavigationImport, 0, len(imports))
	for _, item := range imports {
		if len(languages) == 0 || languages[item.Language] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterNavigationExports(exports []parser.NavigationExport, languages map[string]bool) []parser.NavigationExport {
	filtered := make([]parser.NavigationExport, 0, len(exports))
	for _, export := range exports {
		if len(languages) == 0 || languages[export.Language] {
			filtered = append(filtered, export)
		}
	}
	return filtered
}

func filterNavigationFields(fields []parser.NavigationField, languages map[string]bool) []parser.NavigationField {
	filtered := make([]parser.NavigationField, 0, len(fields))
	for _, field := range fields {
		if len(languages) == 0 || languages[field.Language] {
			filtered = append(filtered, field)
		}
	}
	return filtered
}

func filterNavigationTypeUsages(usages []parser.NavigationTypeUsage, declarations, languages map[string]bool) []parser.NavigationTypeUsage {
	filtered := make([]parser.NavigationTypeUsage, 0, len(usages))
	for _, usage := range usages {
		if declarations[usage.CallerID] && (len(languages) == 0 || languages[usage.Language]) {
			filtered = append(filtered, usage)
		}
	}
	return filtered
}

func filterNavigationMemberAccesses(accesses []parser.NavigationMemberAccess, declarations, languages map[string]bool) []parser.NavigationMemberAccess {
	filtered := make([]parser.NavigationMemberAccess, 0, len(accesses))
	for _, access := range accesses {
		if declarations[access.CallerID] && (len(languages) == 0 || languages[access.Language]) {
			filtered = append(filtered, access)
		}
	}
	return filtered
}

func filterNavigationTargetIDs(ids []string, declarations map[string]bool) []string {
	if len(ids) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if declarations[id] {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func sortedFilterKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}
