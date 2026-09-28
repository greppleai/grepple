package extract

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/navigation/rustmodule"
	codeparser "github.com/greppleai/grepple/internal/parser"
)

type rustAnalysis struct {
	result          *Analysis
	sources         []Source
	implementations []rustImplementation
	declarations    []rustDeclarationRef
}

type rustDeclarationRef struct {
	declaration *Declaration
	path        string
	scope       string
	name        string
	visibility  string
}

type rustImplementation struct {
	path       string
	scope      string
	target     string
	targetPath string
	trait      string
	members    []Member
}

func rustLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info: Language{
			ID: "rust", Extensions: parserLanguageExtensions("rust"),
			FocusedStructure: true, FocusedFlow: true,
		},
		flowIndex:  moduleFocusedFlowIndex{},
		classIndex: moduleFocusedClassIndex{},
		acceptsSource: func(path string) bool {
			return codeparser.NewParser().LanguageFor(path) == "rust"
		},
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareRustModules(result, sources)
			return &rustAnalysis{result: result, sources: append([]Source(nil), sources...)}
		},
		nearestProjectRoot: nearestRustRoot,
		sourceScope: func(source Source) (string, error) {
			return absolutePath(source.Path), nil
		},
		normalizeType:     normalizeRustType,
		generateStructure: generateModuleClass,
		generateFlow:      generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func nearestRustRoot(directory string) string {
	for current := directory; ; current = filepath.Dir(current) {
		if fileExists(filepath.Join(current, "Cargo.toml")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func prepareRustModules(analysis *Analysis, sources []Source) {
	prepareModulePaths(analysis, sources, "rust", nearestRustRoot)
}

func normalizeRustType(value string) string {
	return normalizeMermaidGeneric(strings.ReplaceAll(strings.TrimSpace(value), "::", "."))
}

func (analysis *rustAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	analyzer := rustSourceAnalyzer{analysis: analysis, source: source, moduleID: absolutePath(source.Path)}
	if err := document.Read(func(view codeparser.DocumentView) error {
		for _, node := range view.Root().NamedChildren() {
			analyzer.analyzeItem(node)
		}
		return nil
	}); err != nil {
		return err
	}
	graph := codeparser.NewParser().NavigationGraph(document, source.Path)
	addModuleNavigationSymbols(analysis.result, graph, "rust", analyzer.moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}

func (analysis *rustAnalysis) Finalize() error {
	analysis.attachImplementations()
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

func (analysis *rustAnalysis) attachImplementations() {
	paths := make([]string, 0, len(analysis.sources))
	for _, source := range analysis.sources {
		paths = append(paths, source.Path)
	}
	modules := rustmodule.BuildRustModuleIndex(analysis.result.Navigation, paths)
	for _, implementation := range analysis.implementations {
		declaration := analysis.rustImplementationDeclaration(modules, implementation)
		if declaration == nil {
			continue
		}
		for _, member := range implementation.members {
			declaration.Members = appendRustMember(declaration.Members, member)
		}
		if implementation.trait != "" {
			declaration.Implements[implementation.trait] = true
		}
	}
}

func (analysis *rustAnalysis) rustImplementationDeclaration(modules *rustmodule.RustModuleIndex, implementation rustImplementation) *Declaration {
	moduleKeys := analysis.rustImplementationModuleKeys(modules, implementation)
	if len(moduleKeys) == 0 && (strings.Contains(implementation.targetPath, "::") || analysis.rustImplementationHasImport(implementation)) {
		return nil
	}
	candidates := analysis.rustImplementationCandidates(modules, implementation, moduleKeys)
	if len(candidates) != 1 {
		return nil
	}
	return candidates[0]
}

func (analysis *rustAnalysis) rustImplementationCandidates(modules *rustmodule.RustModuleIndex, implementation rustImplementation, moduleKeys []string) []*Declaration {
	candidates := []*Declaration{}
	for _, reference := range analysis.declarations {
		if reference.name == implementation.target && rustDeclarationMatchesModules(modules, reference, implementation, moduleKeys) {
			candidates = append(candidates, reference.declaration)
		}
	}
	return candidates
}

func rustDeclarationMatchesModules(modules *rustmodule.RustModuleIndex, reference rustDeclarationRef, implementation rustImplementation, moduleKeys []string) bool {
	if len(moduleKeys) == 0 {
		return reference.path == implementation.path && reference.scope == implementation.scope
	}
	sourceKeys := modules.ModuleKeys(implementation.path, implementation.scope)
	if len(sourceKeys) != 1 {
		return false
	}
	matchedKey := ""
	for _, key := range modules.ModuleKeys(reference.path, reference.scope) {
		if !rustStringSliceContains(moduleKeys, key) {
			continue
		}
		if matchedKey != "" && matchedKey != key {
			return false
		}
		matchedKey = key
	}
	return matchedKey != "" && rustmodule.RustItemVisibleFrom(matchedKey, sourceKeys[0], reference.visibility)
}

func (analysis *rustAnalysis) rustImplementationModuleKeys(modules *rustmodule.RustModuleIndex, implementation rustImplementation) []string {
	path := implementation.targetPath
	if strings.HasPrefix(path, "crate::") || strings.HasPrefix(path, "self::") || strings.HasPrefix(path, "super::") {
		return rustModuleKeys(modules.ResolveItemModulesFrom(implementation.path, implementation.scope, codeparser.RustScopedPath(path, implementation.scope)))
	}
	if !strings.Contains(path, "::") {
		imports := []codeparser.NavigationImport{}
		for _, item := range analysis.result.Navigation.Imports {
			if item.Path == implementation.path && item.Scope == implementation.scope && item.Kind == "" && item.Alias == path {
				imports = append(imports, item)
			}
		}
		if len(imports) == 1 {
			return rustModuleKeys(modules.ResolveImportFrom(implementation.path, implementation.scope, imports[0].ImportPath))
		}
		if len(imports) > 0 {
			return nil
		}
		return modules.ModuleKeys(implementation.path, implementation.scope)
	}
	return nil
}

func (analysis *rustAnalysis) rustImplementationHasImport(implementation rustImplementation) bool {
	for _, item := range analysis.result.Navigation.Imports {
		if item.Path == implementation.path && item.Scope == implementation.scope && item.Kind == "" && item.Alias == implementation.targetPath {
			return true
		}
	}
	return false
}

func rustModuleKeys(targets []rustmodule.RustModuleTarget) []string {
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		key := rustmodule.RustModuleTargetModuleKey(target)
		if !rustStringSliceContains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}

func rustStringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type rustSourceAnalyzer struct {
	analysis *rustAnalysis
	source   Source
	moduleID string
	scope    string
}

func (analyzer *rustSourceAnalyzer) analyzeItem(node codeparser.ViewNode) {
	switch node.Kind() {
	case "struct_item":
		analyzer.analyzeStruct(node)
	case "enum_item":
		analyzer.analyzeEnum(node)
	case "trait_item":
		analyzer.analyzeTrait(node)
	case "impl_item":
		analyzer.analyzeImpl(node)
	case "mod_item":
		name := node.ChildByFieldName("name").Text()
		body := node.ChildByFieldName("body")
		if name != "" && body.Valid() {
			nested := *analyzer
			nested.scope = rustJoinScope(analyzer.scope, name)
			for _, item := range body.NamedChildren() {
				nested.analyzeItem(item)
			}
		}
	}
}

func (analyzer *rustSourceAnalyzer) analyzeStruct(node codeparser.ViewNode) {
	declaration := analyzer.newDeclaration(node, "class")
	if declaration == nil {
		return
	}
	declaration.Members = analyzer.rustStructMembers(node.ChildByFieldName("body"))
	analyzer.storeDeclaration(declaration, node)
}

func (analyzer *rustSourceAnalyzer) rustStructMembers(body codeparser.ViewNode) []Member {
	members := []Member{}
	if body.Kind() == "field_declaration_list" {
		for _, field := range body.NamedChildren() {
			if member, ok := analyzer.rustNamedField(field); ok {
				members = appendRustMember(members, member)
			}
		}
		return members
	}
	if body.Kind() != "ordered_field_declaration_list" {
		return members
	}
	visibility := "private"
	index := 0
	for _, field := range body.NamedChildren() {
		switch field.Kind() {
		case "attribute_item":
			continue
		case "visibility_modifier":
			visibility = rustVisibilityModifier(field.Text())
			continue
		}
		members = appendRustMember(members, Member{
			Kind: "property", Name: rustTupleFieldName(index), Type: normalizeRustType(field.Text()), Visibility: visibility,
			Language: "rust", ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(field),
		})
		index++
		visibility = "private"
	}
	return members
}

func (analyzer *rustSourceAnalyzer) analyzeEnum(node codeparser.ViewNode) {
	declaration := analyzer.newDeclaration(node, "class")
	if declaration == nil {
		return
	}
	for _, variant := range node.ChildByFieldName("body").NamedChildren() {
		if variant.Kind() != "enum_variant" {
			continue
		}
		name := variant.ChildByFieldName("name").Text()
		if name == "" {
			continue
		}
		declaration.Members = appendRustMember(declaration.Members, Member{
			Kind: "property", Name: name, Type: declaration.Name, Visibility: "public", Static: true,
			Language: "rust", ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(variant),
		})
	}
	analyzer.storeDeclaration(declaration, node)
}

func (analyzer *rustSourceAnalyzer) analyzeTrait(node codeparser.ViewNode) {
	declaration := analyzer.newDeclaration(node, "interface")
	if declaration == nil {
		return
	}
	collectRustTraitBounds(declaration, node.ChildByFieldName("bounds"))
	for _, item := range node.ChildByFieldName("body").NamedChildren() {
		if member, ok := analyzer.rustTraitMember(item); ok {
			declaration.Members = appendRustMember(declaration.Members, member)
		}
	}
	analyzer.storeDeclaration(declaration, node)
}

func collectRustTraitBounds(declaration *Declaration, bounds codeparser.ViewNode) {
	for _, bound := range bounds.NamedChildren() {
		if bound.Kind() == "lifetime" {
			continue
		}
		if name := rustTypeName(bound); name != "" {
			declaration.Extends[name] = true
		}
	}
}

func (analyzer *rustSourceAnalyzer) rustTraitMember(item codeparser.ViewNode) (Member, bool) {
	switch item.Kind() {
	case "function_item", "function_signature_item":
		return analyzer.rustFunctionMember(item, "public"), true
	case "type_item", "associated_type":
		name := item.ChildByFieldName("name").Text()
		if name == "" {
			return Member{}, false
		}
		typeName := normalizeRustType(item.ChildByFieldName("type").Text())
		if typeName == "" {
			typeName = "associated_type"
		}
		return Member{Kind: "property", Name: name, Type: typeName, Visibility: "public", Language: "rust", ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(item)}, true
	default:
		return Member{}, false
	}
}

func (analyzer *rustSourceAnalyzer) analyzeImpl(node codeparser.ViewNode) {
	target := rustTypeName(node.ChildByFieldName("type"))
	if target == "" {
		return
	}
	implementation := rustImplementation{path: analyzer.source.Path, scope: analyzer.scope, target: target, targetPath: rustTypePath(node.ChildByFieldName("type")), trait: rustTypeName(node.ChildByFieldName("trait"))}
	for _, item := range node.ChildByFieldName("body").NamedChildren() {
		if item.Kind() == "function_item" || item.Kind() == "function_signature_item" {
			implementation.members = append(implementation.members, analyzer.rustFunctionMember(item, "private"))
		}
	}
	analyzer.analysis.implementations = append(analyzer.analysis.implementations, implementation)
}

func (analyzer *rustSourceAnalyzer) newDeclaration(node codeparser.ViewNode, kind string) *Declaration {
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return nil
	}
	return &Declaration{
		Name: name, Kind: kind, Language: "rust", ModuleID: analyzer.moduleID,
		File: analyzer.stablePath(), Location: analyzer.location(node), Extends: map[string]bool{}, Implements: map[string]bool{},
	}
}

func (analyzer *rustSourceAnalyzer) storeDeclaration(declaration *Declaration, node codeparser.ViewNode) {
	key := analyzer.moduleID + ":" + declaration.Name
	if analyzer.scope != "" {
		key = analyzer.moduleID + ":" + analyzer.scope + "::" + declaration.Name
	}
	if analyzer.analysis.result.ModuleDeclarations[key] != nil {
		analyzer.analysis.result.duplicateErrors = append(analyzer.analysis.result.duplicateErrors, "duplicate Rust type "+declaration.Name+" in "+analyzer.source.Path)
		return
	}
	analyzer.analysis.result.ModuleDeclarations[key] = declaration
	analyzer.analysis.declarations = append(analyzer.analysis.declarations, rustDeclarationRef{declaration: declaration, path: analyzer.source.Path, scope: analyzer.scope, name: declaration.Name, visibility: rustItemVisibility(node)})
	if analyzer.analysis.result.ModuleSymbols[key] == nil {
		analyzer.analysis.result.ModuleSymbols[key] = &Symbol{
			Name: declaration.Name, Kind: declaration.Kind, Language: "rust", ModuleID: analyzer.moduleID, Key: key, Calls: map[string]bool{},
			Locations: []Location{analyzer.location(node)},
		}
	}
}

func (analyzer *rustSourceAnalyzer) rustNamedField(node codeparser.ViewNode) (Member, bool) {
	if node.Kind() != "field_declaration" {
		return Member{}, false
	}
	name := node.ChildByFieldName("name").Text()
	typeName := normalizeRustType(node.ChildByFieldName("type").Text())
	if name == "" || typeName == "" {
		return Member{}, false
	}
	return Member{
		Kind: "property", Name: name, Type: typeName, Visibility: rustVisibility(node, "private"),
		Language: "rust", ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(node),
	}, true
}

func (analyzer *rustSourceAnalyzer) rustFunctionMember(node codeparser.ViewNode, fallbackVisibility string) Member {
	parameters := []string{}
	static := true
	for _, parameter := range node.ChildByFieldName("parameters").NamedChildren() {
		switch parameter.Kind() {
		case "self_parameter":
			static = false
		case "parameter":
			if typeName := normalizeRustType(parameter.ChildByFieldName("type").Text()); typeName != "" {
				parameters = append(parameters, typeName)
			}
		case "variadic_parameter":
			parameters = append(parameters, "...")
		}
	}
	return Member{
		Kind: "method", Name: node.ChildByFieldName("name").Text(), Visibility: rustVisibility(node, fallbackVisibility),
		Type: normalizeRustType(node.ChildByFieldName("return_type").Text()), Parameters: parameters, Static: static,
		Async:    rustFunctionAsync(node),
		Language: "rust", ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(node),
	}
}

func rustVisibility(node codeparser.ViewNode, fallback string) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "visibility_modifier" {
			continue
		}
		return rustVisibilityModifier(child.Text())
	}
	return fallback
}

func rustItemVisibility(node codeparser.ViewNode) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "visibility_modifier" {
			return strings.Join(strings.Fields(child.Text()), "")
		}
	}
	return ""
}

func rustVisibilityModifier(value string) string {
	if strings.TrimSpace(value) == "pub" {
		return "public"
	}
	return "package"
}

func rustFunctionAsync(node codeparser.ViewNode) bool {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "function_modifiers" && rustHasWord(child.Text(), "async") {
			return true
		}
	}
	return false
}

func rustHasWord(value, expected string) bool {
	for _, word := range strings.FieldsFunc(value, func(character rune) bool {
		return character == ' ' || character == '\t' || character == '\n' || character == '\r' || character == '(' || character == ')'
	}) {
		if word == expected {
			return true
		}
	}
	return false
}

func rustTypeName(node codeparser.ViewNode) string {
	if !node.Valid() {
		return ""
	}
	switch node.Kind() {
	case "type_identifier", "identifier":
		return node.Text()
	case "generic_type":
		return rustTypeName(node.ChildByFieldName("type"))
	case "scoped_type_identifier":
		return rustTypeName(node.ChildByFieldName("name"))
	}
	if name := rustTypeName(node.ChildByFieldName("name")); name != "" {
		return name
	}
	for _, child := range node.NamedChildren() {
		if name := rustTypeName(child); name != "" {
			return name
		}
	}
	return ""
}

func rustTypePath(node codeparser.ViewNode) string {
	if !node.Valid() {
		return ""
	}
	switch node.Kind() {
	case "type_identifier", "identifier":
		return strings.TrimSpace(node.Text())
	case "generic_type":
		return rustTypePath(node.ChildByFieldName("type"))
	case "scoped_type_identifier":
		path := strings.Join(strings.Fields(node.Text()), "")
		if !strings.ContainsAny(path, "<>()[]{};,") {
			return path
		}
	}
	if path := rustTypePath(node.ChildByFieldName("name")); path != "" {
		return path
	}
	return ""
}

func rustJoinScope(scope, name string) string {
	if scope == "" {
		return name
	}
	return scope + "::" + name
}

func appendRustMember(members []Member, addition Member) []Member {
	if addition.Name == "" {
		return members
	}
	for index := range members {
		if members[index].Name == addition.Name && members[index].Kind == addition.Kind {
			return members
		}
	}
	return append(members, addition)
}

func rustTupleFieldName(index int) string {
	return "_" + strconv.Itoa(index)
}

func (analyzer *rustSourceAnalyzer) stablePath() string {
	return analyzer.analysis.result.SourcePaths[absolutePath(analyzer.source.Path)]
}

func (analyzer *rustSourceAnalyzer) location(node codeparser.ViewNode) Location {
	location := syntaxLocation(analyzer.source.Path, node)
	location.Path = analyzer.stablePath()
	return location
}
