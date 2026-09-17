package extract

import (
	"path"
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

func addModuleNavigationSymbols(analysis *Analysis, graph codeparser.NavigationGraph, language, moduleID, sourcePath string) {
	for _, declaration := range graph.Declarations {
		key := moduleID + ":" + declaration.Name
		if analysis.TSSymbolIndex[key] != nil {
			continue
		}
		analysis.TSSymbolIndex[key] = &Symbol{
			Name: declaration.Name, Kind: declaration.Kind, Language: language, ModuleID: moduleID, Key: key,
			Owner: declaration.Container, NavigationID: declaration.ID, Calls: map[string]bool{},
			Locations: []Location{{Path: sourcePath, Line: declaration.Start, EndLine: declaration.End}},
		}
	}
}

func navigationDeclarationSymbol(analysis *Analysis, declaration *codeparser.NavigationDeclaration) *Symbol {
	var symbols map[string]*Symbol
	switch declaration.Language {
	case "go":
		symbols = analysis.GoSymbolIndex
	case "javascript", "typescript", "tsx", "python", "java", "kotlin", "csharp", "rust", "c", "cpp":
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
	target := analysis.navigationSymbols[call.TargetID]
	if target == nil {
		target = resolveNavigationCallTarget(analysis, owner, call)
	}
	if target == nil || target.NavigationID == "" {
		return
	}
	if call.TargetID == "" {
		call.TargetID = target.NavigationID
	}
	if call.Confidence == "" {
		call.Confidence = navigationResolutionConfidence(owner, call, target)
	}
	analysis.navigationCalls[call.CallerID] = append(analysis.navigationCalls[call.CallerID], target)
}

func resolveNavigationCallTarget(analysis *Analysis, owner *Symbol, call *codeparser.NavigationCall) *Symbol {
	if target := resolveNavigationFactoryReturnTarget(analysis, owner, call); target != nil {
		return target
	}
	if call.ReceiverType != "" {
		if target := resolveNavigationTypedReceiver(analysis, owner, call.ReceiverType, call.Name, call.ImportPath); target != nil {
			return target
		}
	}
	if owner.Language == "go" {
		return resolveGoCall(analysis, owner.PackageID, call.ResolvedName)
	}
	if owner.Language == "javascript" || owner.Language == "typescript" || owner.Language == "tsx" {
		return resolveTypeScriptCall(analysis, owner, call.ResolvedName)
	}
	if owner.Language == "python" {
		return resolvePythonNavigationCall(analysis, owner, call.ResolvedName)
	}
	if owner.Language == "c" || owner.Language == "cpp" {
		return resolveCFamilyNavigationCall(analysis, owner, call.ResolvedName)
	}
	if owner.Language == "java" || owner.Language == "kotlin" || owner.Language == "csharp" || owner.Language == "rust" {
		return resolveJVMNavigationCall(analysis, owner, call.ResolvedName)
	}
	return nil
}

func resolvePythonNavigationCall(analysis *Analysis, owner *Symbol, name string) *Symbol {
	if local := analysis.TSSymbolIndex[owner.ModuleID+":"+name]; local != nil {
		return local
	}
	terminal := name
	if index := strings.LastIndex(terminal, "."); index >= 0 {
		terminal = terminal[index+1:]
	}
	var found *Symbol
	for _, key := range sortedKeys(analysis.TSSymbolIndex) {
		candidate := analysis.TSSymbolIndex[key]
		candidateTerminal := candidate.Name
		if index := strings.LastIndex(candidateTerminal, "."); index >= 0 {
			candidateTerminal = candidateTerminal[index+1:]
		}
		if candidate.Language != "python" || candidateTerminal != terminal {
			continue
		}
		if found != nil && found.Key != candidate.Key {
			return nil
		}
		found = candidate
	}
	return found
}

func resolveJVMNavigationCall(analysis *Analysis, owner *Symbol, name string) *Symbol {
	candidates := []string{name}
	if owner.Owner != "" && !strings.Contains(name, ".") {
		candidates = append(candidates, owner.Owner+"."+name)
	}
	if !strings.Contains(name, ".") {
		candidates = append(candidates, name+"."+name)
	}
	for _, candidateName := range candidates {
		candidate := analysis.TSSymbolIndex[owner.ModuleID+":"+candidateName]
		if candidate != nil && candidate.NavigationID != "" {
			return candidate
		}
	}
	terminal := name
	if index := strings.LastIndex(terminal, "."); index >= 0 {
		terminal = terminal[index+1:]
	}
	var found *Symbol
	for _, key := range sortedKeys(analysis.TSSymbolIndex) {
		candidate := analysis.TSSymbolIndex[key]
		if candidate.Language != owner.Language || candidate.NavigationID == "" || navigationTerminal(candidate.Name) != terminal {
			continue
		}
		if found != nil && found.Key != candidate.Key {
			return nil
		}
		found = candidate
	}
	return found
}

func resolveCFamilyNavigationCall(analysis *Analysis, owner *Symbol, name string) *Symbol {
	name = strings.ReplaceAll(name, "::", ".")
	terminal := navigationTerminal(name)
	if owner.Owner != "" {
		if target := uniqueCFamilyNavigationSymbol(analysis, owner.Language, owner.ModuleID, owner.Owner+"."+terminal); target != nil {
			return target
		}
	}
	if target := uniqueCFamilyNavigationSymbol(analysis, owner.Language, owner.ModuleID, name); target != nil {
		return target
	}
	return uniqueCFamilyNavigationSymbol(analysis, owner.Language, "", terminal)
}

func uniqueCFamilyNavigationSymbol(analysis *Analysis, language, moduleID, name string) *Symbol {
	var found *Symbol
	for _, key := range sortedKeys(analysis.TSSymbolIndex) {
		candidate := analysis.TSSymbolIndex[key]
		if candidate.Language != language || candidate.NavigationID == "" || moduleID != "" && candidate.ModuleID != moduleID {
			continue
		}
		candidateName := candidate.Name
		if moduleID == "" {
			candidateName = navigationTerminal(candidateName)
		}
		if candidateName != name {
			continue
		}
		if found != nil {
			return nil
		}
		found = candidate
	}
	return found
}

func navigationTerminal(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func resolveNavigationFactoryReturnTarget(analysis *Analysis, owner *Symbol, call *codeparser.NavigationCall) *Symbol {
	if call.ReceiverFactory == "" {
		return nil
	}
	factory := resolveNavigationCallableInScope(analysis, owner, call.ReceiverFactory, call.ReceiverFactoryImport)
	if factory == nil || factory.NavigationID == "" {
		return nil
	}
	declaration := navigationDeclarationByID(analysis, factory.NavigationID)
	if declaration == nil || declaration.ResultType == "" {
		return nil
	}
	call.ReceiverType = declaration.ResultType
	call.ImportPath = declaration.ResultImportPath
	if call.ImportPath == "" {
		call.ImportPath = call.ReceiverFactoryImport
	}
	return resolveNavigationTypedReceiver(analysis, factory, declaration.ResultType, call.Name, declaration.ResultImportPath)
}

func resolveNavigationCallableInScope(analysis *Analysis, owner *Symbol, name, importPath string) *Symbol {
	if importPath == "" {
		if owner.Language == "go" {
			return analysis.GoSymbolIndex[owner.PackageID+":"+name]
		}
		return analysis.TSSymbolIndex[owner.ModuleID+":"+name]
	}
	if owner.Language == "go" {
		return resolveImportedGoNavigationSymbol(analysis, importPath, name)
	}
	moduleID := resolveTypeScriptNavigationModule(analysis, owner.ModuleID, importPath)
	return analysis.TSSymbolIndex[moduleID+":"+name]
}

func resolveNavigationTypedReceiver(analysis *Analysis, owner *Symbol, typeName, method, importPath string) *Symbol {
	name := typeName + "." + method
	if importPath == "" {
		if owner.Language == "go" {
			return analysis.GoSymbolIndex[owner.PackageID+":"+name]
		}
		return analysis.TSSymbolIndex[owner.ModuleID+":"+name]
	}
	if owner.Language == "go" {
		return resolveImportedGoNavigationSymbol(analysis, importPath, name)
	}
	moduleID := resolveTypeScriptNavigationModule(analysis, owner.ModuleID, importPath)
	return analysis.TSSymbolIndex[moduleID+":"+name]
}

func resolveImportedGoNavigationSymbol(analysis *Analysis, importPath, name string) *Symbol {
	if packageID := analysis.GoImportPathIndex[importPath]; packageID != "" {
		return analysis.GoSymbolIndex[packageID+":"+name]
	}
	return uniqueImportedGoSymbol(analysis, path.Base(importPath), name)
}

func resolveTypeScriptNavigationModule(analysis *Analysis, importerModuleID, importPath string) string {
	for _, binding := range analysis.TSImportBindings[importerModuleID] {
		if binding.Source == importPath {
			return binding.ModuleID
		}
	}
	return ""
}

func navigationDeclarationByID(analysis *Analysis, id string) *codeparser.NavigationDeclaration {
	for index := range analysis.Navigation.Declarations {
		if analysis.Navigation.Declarations[index].ID == id {
			return &analysis.Navigation.Declarations[index]
		}
	}
	return nil
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
	if (owner.Language == "javascript" || owner.Language == "typescript" || owner.Language == "tsx") && parts[0] == "this" && owner.Owner != "" {
		return owner.Owner + "." + parts[1]
	}
	if owner.Language == "python" && (parts[0] == "self" || parts[0] == "cls") && owner.Owner != "" {
		return owner.Owner + "." + parts[1]
	}
	if (owner.Language == "java" || owner.Language == "kotlin" || owner.Language == "csharp") && parts[0] == "this" && owner.Owner != "" {
		return owner.Owner + "." + parts[1]
	}
	return display
}

func navigationResolutionConfidence(owner *Symbol, call *codeparser.NavigationCall, target *Symbol) string {
	if (owner.Language == "python" || owner.Language == "java" || owner.Language == "kotlin" || owner.Language == "csharp" || owner.Language == "c" || owner.Language == "cpp") && navigationSymbolScope(owner) != navigationSymbolScope(target) {
		return "unique-terminal"
	}
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
