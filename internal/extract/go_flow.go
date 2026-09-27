package extract

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

func hasGoOrderedPhasePath(analysis *Analysis, source, target *Symbol) bool {
	for _, key := range sortedKeys(analysis.PackageSymbols) {
		calls := resolvedGoCalls(analysis, analysis.PackageSymbols[key])
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
