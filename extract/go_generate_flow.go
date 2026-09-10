package extract

import (
	"fmt"
	"strings"
)

type selectedGoSymbol = selectedNavigationSymbol

func generateGoFlowchart(entry string, entrySource Source, sources []Source, depth, nodeLimit int) (string, error) {
	analysis, err := Analyze(sources)
	if err != nil {
		return "", err
	}
	entrySymbol := goEntrySymbol(analysis, entry, entrySource.Path)
	if entrySymbol == nil {
		return "", fmt.Errorf("Function or method '%s' was not found in entry file %s", entry, entrySource.Path)
	}
	selected, truncated := selectGoSymbols(analysis, entrySymbol, depth, nodeLimit)
	diagram, err := renderGoFlow(entry, selected, analysis, depth, nodeLimit, truncated)
	if err != nil {
		return "", err
	}
	diagnostics, err := CheckFlowchartWithAnalysis(diagram, analysis)
	if err != nil {
		return "", err
	}
	if len(diagnostics) > 0 {
		return "", fmt.Errorf("Generated flowchart failed validation:\n%s", formatDiagnostics(diagnostics))
	}
	return diagram, nil
}

func goEntrySymbol(analysis *Analysis, name, entryPath string) *Symbol {
	for _, key := range sortedKeys(analysis.GoSymbolIndex) {
		symbol := analysis.GoSymbolIndex[key]
		if symbol.Name != name {
			continue
		}
		for _, location := range symbol.Locations {
			if absolutePath(location.Path) == absolutePath(entryPath) {
				return symbol
			}
		}
	}
	return nil
}

func selectGoSymbols(analysis *Analysis, entry *Symbol, depth, nodeLimit int) ([]selectedGoSymbol, bool) {
	return selectNavigationSymbols(analysis, entry, depth, nodeLimit)
}

func renderGoFlow(entry string, selected []selectedGoSymbol, analysis *Analysis, depth, nodeLimit int, truncated bool) (string, error) {
	identifiers := goFlowNodeIDs(selected)
	lines := []string{"flowchart TD", fmt.Sprintf("    %%%% grepple:generated entry %s depth %d max-nodes %d", entry, depth, nodeLimit)}
	if truncated {
		lines = append(lines, fmt.Sprintf("    %%%% grepple:truncated max-nodes %d", nodeLimit))
	}
	for _, item := range selected {
		label := flowLabel(item.symbol.Name)
		if location := symbolFlowLocation(item.symbol, analysis); location != "" {
			label += " · " + location
		}
		lines = append(lines, fmt.Sprintf("    %s[\"%s\"]", identifiers[item.symbol.Key], label))
	}
	lines = append(lines, "")
	for _, item := range selected {
		lines = append(lines, goFlowMetadata(item.symbol, identifiers[item.symbol.Key], analysis)...)
	}
	lines = append(lines, "")
	edges, err := goFlowEdges(selected, identifiers, analysis, depth, truncated)
	if err != nil {
		return "", err
	}
	lines = append(lines, edges...)
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n", nil
}

func symbolFlowLocation(symbol *Symbol, analysis *Analysis) string {
	if len(symbol.Locations) == 0 {
		return ""
	}
	location := symbol.Locations[0]
	if path := analysis.SourcePaths[absolutePath(location.Path)]; path != "" {
		location.Path = path
	}
	return sourceRange(location)
}

func goFlowNodeIDs(selected []selectedGoSymbol) map[string]string {
	result, used := map[string]string{}, map[string]bool{}
	for _, item := range selected {
		base := flowIDBase(item.symbol.Name)
		identifier := base
		for suffix := 2; used[identifier]; suffix++ {
			identifier = fmt.Sprintf("%s_%d", base, suffix)
		}
		used[identifier] = true
		result[item.symbol.Key] = identifier
	}
	return result
}

func goFlowMetadata(symbol *Symbol, identifier string, analysis *Analysis) []string {
	scope := goPackageScope(symbol.PackageID, symbol.Package, analysis)
	result := []string{fmt.Sprintf("    %%%% grepple:symbol %s %s", identifier, symbol.Name), fmt.Sprintf("    %%%% grepple:language %s go", identifier)}
	return append(result, fmt.Sprintf("    %%%% grepple:package %s %s", identifier, scope))
}

func goFlowEdges(selected []selectedGoSymbol, identifiers map[string]string, analysis *Analysis, depth int, truncated bool) ([]string, error) {
	chosen, edges := map[string]bool{}, map[string]bool{}
	for _, item := range selected {
		chosen[item.symbol.Key] = true
	}
	for _, item := range selected {
		if err := addGoFlowEdges(item, identifiers, chosen, edges, analysis, depth, truncated); err != nil {
			return nil, err
		}
	}
	return sortedKeys(edges), nil
}

func addGoFlowEdges(item selectedGoSymbol, identifiers map[string]string, chosen, edges map[string]bool, analysis *Analysis, depth int, truncated bool) error {
	for _, called := range resolvedGoCalls(analysis, item.symbol) {
		if !chosen[called.Key] {
			if item.depth < depth && !truncated {
				return fmt.Errorf("generated Go flow omitted resolvable call from %s to %s", item.symbol.Name, called.Name)
			}
			continue
		}
		edges[fmt.Sprintf("    %s --> %s", identifiers[item.symbol.Key], identifiers[called.Key])] = true
	}
	return nil
}
