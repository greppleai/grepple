package extract

import (
	"fmt"
	"strings"
)

type typeScriptFlowItem struct {
	key   string
	depth int
}

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
	selected := selectTypeScriptSymbols(entryKey, analysis, depth, limit)
	identifiers := map[string]string{}
	used := map[string]bool{}
	for _, key := range selected {
		identifiers[key] = uniqueFlowIdentifier(analysis.TSSymbolIndex[key].Name, used)
	}
	lines := []string{"flowchart TD", fmt.Sprintf("    %%%% grepple:generated entry %s depth %d max-nodes %d", entry, depth, limit)}
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
		for _, call := range analysis.TSSymbolIndex[key].CallOrder {
			target := resolveTypeScriptCall(analysis, analysis.TSSymbolIndex[key], call)
			if target != nil && chosen[target.Key] {
				edges[fmt.Sprintf("    %s --> %s", identifiers[key], identifiers[target.Key])] = true
			}
		}
	}
	return sortedKeys(edges)
}

func selectTypeScriptSymbols(entry string, analysis *Analysis, depth, limit int) []string {
	pending := []typeScriptFlowItem{{entry, 0}}
	seen := map[string]bool{}
	result := []string{}
	for len(pending) > 0 && len(result) < limit {
		item := pending[0]
		pending = pending[1:]
		symbol := analysis.TSSymbolIndex[item.key]
		if symbol == nil || seen[item.key] {
			continue
		}
		seen[item.key], result = true, append(result, item.key)
		if item.depth >= depth {
			continue
		}
		for _, call := range symbol.CallOrder {
			if target := resolveTypeScriptCall(analysis, symbol, call); target != nil && !seen[target.Key] {
				pending = append(pending, typeScriptFlowItem{target.Key, item.depth + 1})
			}
		}
	}
	return result
}

func uniqueFlowIdentifier(name string, used map[string]bool) string {
	base, identifier := flowIDBase(name), flowIDBase(name)
	for suffix := 2; used[identifier]; suffix++ {
		identifier = fmt.Sprintf("%s_%d", base, suffix)
	}
	used[identifier] = true
	return identifier
}
