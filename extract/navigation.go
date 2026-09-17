package extract

import codeparser "github.com/greppleai/grepple/parser"

// projectNavigationGraph derives focused-extraction compatibility fields from the
// canonical resolved repository graph. It intentionally performs no resolution.
func projectNavigationGraph(analysis *Analysis) {
	analysis.navigationSymbols = make(map[string]*Symbol)
	analysis.navigationCalls = make(map[string][]*Symbol)
	for index := range analysis.Navigation.Declarations {
		projectNavigationDeclaration(analysis, &analysis.Navigation.Declarations[index])
	}
	for index := range analysis.Navigation.Calls {
		projectNavigationCall(analysis, &analysis.Navigation.Calls[index])
	}
}

func projectNavigationDeclaration(analysis *Analysis, declaration *codeparser.NavigationDeclaration) {
	symbol := navigationDeclarationSymbol(analysis, declaration)
	if symbol == nil {
		return
	}
	if declaration.Scope == "" {
		declaration.Scope = navigationSymbolScope(symbol)
	}
	if declaration.Package == "" {
		declaration.Package = symbol.Package
	}
	if declaration.PackageID == "" {
		declaration.PackageID = symbol.PackageID
	}
	if declaration.ModuleID == "" {
		declaration.ModuleID = symbol.ModuleID
	}
	if declaration.Receiver == "" {
		declaration.Receiver = symbol.Receiver
	}
	if declaration.Container == "" {
		declaration.Container = symbol.Owner
	}
	analysis.navigationSymbols[declaration.ID] = symbol
	symbol.NavigationID = declaration.ID
}

func addModuleNavigationSymbols(analysis *Analysis, graph codeparser.NavigationGraph, language, moduleID, sourcePath string) {
	for _, declaration := range graph.Declarations {
		key := moduleID + ":" + declaration.Name
		if analysis.ModuleSymbols[key] != nil {
			continue
		}
		analysis.ModuleSymbols[key] = &Symbol{
			Name: declaration.Name, Kind: declaration.Kind, Language: language, ModuleID: moduleID, Key: key,
			Owner: declaration.Container, NavigationID: declaration.ID, Calls: map[string]bool{},
			Locations: []Location{{Path: sourcePath, Line: declaration.Start, EndLine: declaration.End}},
		}
	}
}

func navigationDeclarationSymbol(analysis *Analysis, declaration *codeparser.NavigationDeclaration) *Symbol {
	for _, symbols := range []map[string]*Symbol{analysis.PackageSymbols, analysis.ModuleSymbols} {
		for _, key := range sortedKeys(symbols) {
			symbol := symbols[key]
			if symbol.Language == declaration.Language && symbol.Name == declaration.Name && symbolHasNavigationLocation(symbol, declaration) {
				return symbol
			}
		}
	}
	return nil
}

func symbolHasNavigationLocation(symbol *Symbol, declaration *codeparser.NavigationDeclaration) bool {
	for _, location := range symbol.Locations {
		if absolutePath(location.Path) == absolutePath(declaration.Path) && location.Line >= declaration.Start && location.Line <= declaration.End {
			return true
		}
	}
	return false
}

func navigationSymbolScope(symbol *Symbol) string {
	if symbol.PackageID != "" {
		return symbol.PackageID
	}
	return symbol.ModuleID
}

func projectNavigationCall(analysis *Analysis, call *codeparser.NavigationCall) {
	owner := analysis.navigationSymbols[call.CallerID]
	if owner == nil {
		return
	}
	resolvedName := call.ResolvedName
	if resolvedName == "" {
		resolvedName = call.Name
	}
	owner.CallOrder = append(owner.CallOrder, resolvedName)
	owner.Calls[resolvedName] = true
	target := analysis.navigationSymbols[call.TargetID]
	if target != nil {
		analysis.navigationCalls[call.CallerID] = append(analysis.navigationCalls[call.CallerID], target)
	}
}

func resolvedNavigationCalls(analysis *Analysis, symbol *Symbol) []*Symbol {
	if symbol == nil || symbol.NavigationID == "" {
		return nil
	}
	return analysis.navigationCalls[symbol.NavigationID]
}

type selectedNavigationSymbol struct {
	symbol *Symbol
	depth  int
}

func selectNavigationSymbols(analysis *Analysis, entry *Symbol, depth, nodeLimit int) ([]selectedNavigationSymbol, bool) {
	pending := []selectedNavigationSymbol{{symbol: entry}}
	seen, queued := map[string]bool{}, map[string]bool{entry.Key: true}
	var result []selectedNavigationSymbol
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if seen[current.symbol.Key] {
			continue
		}
		if len(result) >= nodeLimit {
			return result, true
		}
		seen[current.symbol.Key] = true
		result = append(result, current)
		if current.depth >= depth {
			continue
		}
		for _, target := range resolvedNavigationCalls(analysis, current.symbol) {
			if !seen[target.Key] && !queued[target.Key] {
				pending = append(pending, selectedNavigationSymbol{symbol: target, depth: current.depth + 1})
				queued[target.Key] = true
			}
		}
	}
	return result, false
}
