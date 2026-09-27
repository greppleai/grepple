package extract

func hasModuleCallPath(analysis *Analysis, source, target *Symbol) bool {
	if source.Key == target.Key {
		return true
	}
	visited := map[string]bool{source.Key: true}
	pending := []*Symbol{source}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		for _, called := range resolvedModuleCalls(analysis, current) {
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

func hasModuleOrderedPath(analysis *Analysis, source, target *Symbol) bool {
	for _, key := range sortedKeys(analysis.ModuleSymbols) {
		owner := analysis.ModuleSymbols[key]
		if owner.ModuleID != source.ModuleID || owner.ModuleID != target.ModuleID {
			continue
		}
		if navigationSymbolsInOrder(resolvedModuleCalls(analysis, owner), source.Key, target.Key) {
			return true
		}
	}
	return false
}

func resolvedModuleCalls(analysis *Analysis, symbol *Symbol) []*Symbol {
	return resolvedNavigationCalls(analysis, symbol)
}

func navigationSymbolsInOrder(calls []*Symbol, sourceKey, targetKey string) bool {
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
