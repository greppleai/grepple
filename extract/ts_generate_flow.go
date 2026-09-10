package extract

import (
	"fmt"
	"strings"
)

func generateTypeScriptFlowchart(entry string, entrySource Source, sources []Source, depth, limit int) (string, error) {
	analysis, err := Analyze(sources)
	if err != nil {
		return "", err
	}
	entryKey := absolutePath(entrySource.Path) + ":" + entry
	entrySymbol := analysis.TSSymbolIndex[entryKey]
	if err := validateFlowEntry(entry, entrySource.Path, entrySymbol); err != nil {
		return "", err
	}
	selected, truncated := selectTypeScriptSymbols(entryKey, analysis, depth, limit)
	identifiers := map[string]string{}
	used := map[string]bool{}
	for _, key := range selected {
		identifiers[key] = uniqueFlowIdentifier(analysis.TSSymbolIndex[key].Name, used)
	}
	lines := []string{"flowchart TD", fmt.Sprintf("    %%%% grepple:generated entry %s depth %d max-nodes %d", entry, depth, limit)}
	if truncated {
		lines = append(lines, fmt.Sprintf("    %%%% grepple:truncated max-nodes %d", limit))
	}
	for _, key := range selected {
		symbol := analysis.TSSymbolIndex[key]
		label := flowLabel(symbol.Name)
		if location := symbolFlowLocation(symbol, analysis); location != "" {
			label += " · " + location
		}
		lines = append(lines, fmt.Sprintf("    %s[\"%s\"]", identifiers[key], label))
	}
	lines = append(lines, "")
	for _, key := range selected {
		symbol := analysis.TSSymbolIndex[key]
		lines = append(lines, fmt.Sprintf("    %%%% grepple:symbol %s %s", identifiers[key], symbol.Name))
		lines = append(lines, fmt.Sprintf("    %%%% grepple:language %s typescript", identifiers[key]))
		lines = append(lines, fmt.Sprintf("    %%%% grepple:module %s %s", identifiers[key], analysis.TSModulePaths[symbol.ModuleID]))
	}
	lines = append(lines, "")
	lines = append(lines, typeScriptFlowEdges(selected, identifiers, analysis)...)
	diagram := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	diagnostics, err := CheckFlowchartWithAnalysis(diagram, analysis)
	if err != nil {
		return "", err
	}
	if len(diagnostics) > 0 {
		return "", fmt.Errorf("Generated flowchart failed validation:\n%s", formatDiagnostics(diagnostics))
	}
	return diagram, nil
}

func typeScriptFlowEdges(selected []string, identifiers map[string]string, analysis *Analysis) []string {
	chosen, edges := stringSet(selected), map[string]bool{}
	for _, key := range selected {
		for _, target := range resolvedTypeScriptCalls(analysis, analysis.TSSymbolIndex[key]) {
			if chosen[target.Key] {
				edges[fmt.Sprintf("    %s --> %s", identifiers[key], identifiers[target.Key])] = true
			}
		}
	}
	return sortedKeys(edges)
}

func selectTypeScriptSymbols(entry string, analysis *Analysis, depth, limit int) ([]string, bool) {
	symbols, truncated := selectNavigationSymbols(analysis, analysis.TSSymbolIndex[entry], depth, limit)
	result := make([]string, 0, len(symbols))
	for _, item := range symbols {
		result = append(result, item.symbol.Key)
	}
	return result, truncated
}

func uniqueFlowIdentifier(name string, used map[string]bool) string {
	base, identifier := flowIDBase(name), flowIDBase(name)
	for suffix := 2; used[identifier]; suffix++ {
		identifier = fmt.Sprintf("%s_%d", base, suffix)
	}
	used[identifier] = true
	return identifier
}
