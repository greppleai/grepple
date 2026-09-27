package extract

type focusedFlowIndex interface {
	symbol(*FlowNode, *Analysis) *Symbol
	symbolCount(*FlowNode, *Analysis) int
}

type packageFocusedFlowIndex struct{}

func (packageFocusedFlowIndex) symbol(node *FlowNode, analysis *Analysis) *Symbol {
	var result *Symbol
	for _, symbol := range analysis.PackageSymbols {
		if symbol.Name != node.Symbol || !packageScopeMatches(node.Package, symbol.Package, symbol.PackageID, analysis) {
			continue
		}
		if result != nil {
			return nil
		}
		result = symbol
	}
	return result
}

func (packageFocusedFlowIndex) symbolCount(node *FlowNode, analysis *Analysis) int {
	count := 0
	for _, symbol := range analysis.PackageSymbols {
		if symbol.Name == node.Symbol && packageScopeMatches(node.Package, symbol.Package, symbol.PackageID, analysis) {
			count++
		}
	}
	return count
}

type moduleFocusedFlowIndex struct{}

func (moduleFocusedFlowIndex) symbol(node *FlowNode, analysis *Analysis) *Symbol {
	if node.Module != "" {
		moduleID := resolveModuleScope(node.Module, analysis)
		if moduleID == "" {
			return nil
		}
		symbol := analysis.ModuleSymbols[moduleID+":"+node.Symbol]
		if symbol != nil && node.Language != "" && symbol.Language != node.Language {
			return nil
		}
		return symbol
	}
	var result *Symbol
	for _, symbol := range analysis.ModuleSymbols {
		if symbol.Name != node.Symbol || node.Language != "" && symbol.Language != node.Language {
			continue
		}
		if result != nil {
			return nil
		}
		result = symbol
	}
	return result
}

func (moduleFocusedFlowIndex) symbolCount(node *FlowNode, analysis *Analysis) int {
	count, moduleID := 0, resolveModuleScope(node.Module, analysis)
	for _, symbol := range analysis.ModuleSymbols {
		if symbol.Name == node.Symbol && (node.Language == "" || symbol.Language == node.Language) && (node.Module == "" || symbol.ModuleID == moduleID) {
			count++
		}
	}
	return count
}

func focusedFlowIndexForNode(node *FlowNode) focusedFlowIndex {
	if node.Package != "" {
		return packageFocusedFlowIndex{}
	}
	if node.Module != "" {
		return moduleFocusedFlowIndex{}
	}
	if node.Language == "" {
		return nil
	}
	definition, ok := languageDefinitionForID(node.Language)
	if !ok {
		return nil
	}
	return definition.flowIndex
}
