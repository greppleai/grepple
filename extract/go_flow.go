package extract

import (
	"path"
	"strings"
)

func hasGoCallPath(analysis *Analysis, source, target *Symbol) bool {
	if source.Key == target.Key {
		return true
	}
	visited := map[string]bool{source.Key: true}
	pending := []*Symbol{source}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		for _, called := range resolvedGoCalls(analysis, current) {
			if called.Key == target.Key {
				return true
			}
			if !visited[called.Key] {
				visited[called.Key] = true
				pending = append(pending, called)
			}
		}
	}
	return false
}

func resolvedGoCalls(analysis *Analysis, symbol *Symbol) []*Symbol {
	return resolvedNavigationCalls(analysis, symbol)
}

func resolveGoCall(analysis *Analysis, packageID, call string) *Symbol {
	if !strings.Contains(call, ".") {
		return analysis.GoSymbolIndex[packageID+":"+call]
	}
	if local := analysis.GoSymbolIndex[packageID+":"+call]; local != nil {
		return local
	}
	parts := strings.SplitN(call, ".", 2)
	module := analysis.GoPackageImports[packageID][parts[0]]
	if module == "" {
		return nil
	}
	if targetPackageID := analysis.GoImportPathIndex[module]; targetPackageID != "" {
		return analysis.GoSymbolIndex[targetPackageID+":"+parts[1]]
	}
	return uniqueImportedGoSymbol(analysis, path.Base(module), parts[1])
}

func uniqueImportedGoSymbol(analysis *Analysis, packageName, symbolName string) *Symbol {
	var result *Symbol
	for packageID, name := range analysis.GoPackageNames {
		if name != packageName {
			continue
		}
		candidate := analysis.GoSymbolIndex[packageID+":"+symbolName]
		if candidate == nil {
			continue
		}
		if result != nil {
			return nil
		}
		result = candidate
	}
	return result
}

func hasGoOrderedPhasePath(analysis *Analysis, source, target *Symbol) bool {
	for _, key := range sortedKeys(analysis.GoSymbolIndex) {
		calls := resolvedGoCalls(analysis, analysis.GoSymbolIndex[key])
		if goSymbolsInOrder(calls, source.Key, target.Key) {
			return true
		}
	}
	return false
}

func goSymbolsInOrder(calls []*Symbol, sourceKey, targetKey string) bool {
	sawSource := false
	for _, call := range calls {
		if call.Key == sourceKey {
			sawSource = true
		} else if sawSource && call.Key == targetKey {
			return true
		}
	}
	return false
}
