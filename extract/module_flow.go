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

func resolveTypeScriptCall(analysis *Analysis, owner *Symbol, call string) *Symbol {
	if local := analysis.ModuleSymbols[owner.ModuleID+":"+call]; local != nil {
		return local
	}
	parts := splitTypeScriptCall(call)
	binding := analysis.ModuleImportBindings[owner.ModuleID][parts[0]]
	if binding.ModuleID == "" {
		return nil
	}
	name := binding.Resolved
	if name == "" {
		return nil
	}
	if parts[1] != "" {
		name += "." + parts[1]
	}
	return analysis.ModuleSymbols[binding.ModuleID+":"+name]
}

func splitTypeScriptCall(call string) [2]string {
	for index, character := range call {
		if character == '.' {
			return [2]string{call[:index], call[index+1:]}
		}
	}
	return [2]string{call, ""}
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
