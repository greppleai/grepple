package extract

import (
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

func enrichNavigationGraph(analysis *Analysis) {
	for index := range analysis.Navigation.Declarations {
		declaration := &analysis.Navigation.Declarations[index]
		symbol := navigationDeclarationSymbol(analysis, declaration)
		if symbol == nil {
			continue
		}
		declaration.Scope = navigationSymbolScope(symbol)
		declaration.Package = symbol.Package
		declaration.PackageID = symbol.PackageID
		declaration.ModuleID = symbol.ModuleID
		declaration.Receiver = symbol.Receiver
		if declaration.Container == "" {
			declaration.Container = symbol.Owner
		}
		analysis.navigationSymbols[declaration.ID] = symbol
		if symbol.NavigationID == "" {
			symbol.NavigationID = declaration.ID
		}
	}
	for index := range analysis.Navigation.Calls {
		enrichNavigationCall(analysis, &analysis.Navigation.Calls[index])
	}
}

func navigationDeclarationSymbol(analysis *Analysis, declaration *codeparser.NavigationDeclaration) *Symbol {
	var symbols map[string]*Symbol
	switch declaration.Language {
	case "go":
		symbols = analysis.GoSymbolIndex
	case "typescript", "tsx":
		symbols = analysis.TSSymbolIndex
	default:
		return nil
	}
	for _, key := range sortedKeys(symbols) {
		symbol := symbols[key]
		if symbol.Name == declaration.Name && symbolHasNavigationLocation(symbol, declaration) {
			return symbol
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
	if symbol.Language == "go" {
		return symbol.PackageID
	}
	return symbol.ModuleID
}

func enrichNavigationCall(analysis *Analysis, call *codeparser.NavigationCall) {
	owner := analysis.navigationSymbols[call.CallerID]
	if owner == nil {
		return
	}
	call.ResolvedName = navigationCallName(owner, call)
	owner.CallOrder = append(owner.CallOrder, call.ResolvedName)
	owner.Calls[call.ResolvedName] = true
	var target *Symbol
	if owner.Language == "go" {
		target = resolveGoCall(analysis, owner.PackageID, call.ResolvedName)
	} else if owner.Language == "typescript" {
		target = resolveTypeScriptCall(analysis, owner, call.ResolvedName)
	}
	if target == nil || target.NavigationID == "" {
		return
	}
	call.TargetID = target.NavigationID
	call.Confidence = navigationResolutionConfidence(owner, call, target)
	analysis.navigationCalls[call.CallerID] = append(analysis.navigationCalls[call.CallerID], target)
}

func navigationCallName(owner *Symbol, call *codeparser.NavigationCall) string {
	display := strings.Join(strings.Fields(call.Display), "")
	if display == "" || strings.ContainsAny(display, "()[]{}<>") {
		return call.Name
	}
	parts := strings.SplitN(display, ".", 2)
	if len(parts) == 1 {
		return call.Name
	}
	if owner.Language == "go" && parts[0] == owner.Receiver && owner.Owner != "" {
		return owner.Owner + "." + parts[1]
	}
	if owner.Language == "typescript" && parts[0] == "this" && owner.Owner != "" {
		return owner.Owner + "." + parts[1]
	}
	return display
}

func navigationResolutionConfidence(owner *Symbol, call *codeparser.NavigationCall, target *Symbol) string {
	if call.Display == target.Name || call.Name == target.Name && !strings.Contains(call.Display, ".") {
		return "exact"
	}
	if navigationSymbolScope(owner) == navigationSymbolScope(target) {
		return "context-resolved"
	}
	return "import-resolved"
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
