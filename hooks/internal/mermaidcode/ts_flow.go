package mermaidcode

func hasTypeScriptCallPath(analysis *Analysis, source, target *Symbol) bool {
	if source.Key == target.Key {
		return true
	}
	visited := map[string]bool{source.Key: true}
	pending := []*Symbol{source}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		for _, call := range current.CallOrder {
			called := resolveTypeScriptCall(analysis, current, call)
			if called == nil {
				continue
			}
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
	if local := analysis.TSSymbolIndex[owner.ModuleID+":"+call]; local != nil {
		return local
	}
	parts := splitTypeScriptCall(call)
	binding := analysis.TSImportBindings[owner.ModuleID][parts[0]]
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
	return analysis.TSSymbolIndex[binding.ModuleID+":"+name]
}

func splitTypeScriptCall(call string) [2]string {
	for index, character := range call {
		if character == '.' {
			return [2]string{call[:index], call[index+1:]}
		}
	}
	return [2]string{call, ""}
}

func hasTypeScriptOrderedPath(analysis *Analysis, source, target *Symbol) bool {
	for _, key := range sortedKeys(analysis.TSSymbolIndex) {
		owner := analysis.TSSymbolIndex[key]
		if owner.ModuleID != source.ModuleID || owner.ModuleID != target.ModuleID {
			continue
		}
		if callsInOrder(owner.CallOrder, source.Name, target.Name) {
			return true
		}
	}
	return false
}
