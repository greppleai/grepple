package extract

import (
	"path/filepath"
	"strconv"
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

type rustAnalysis struct {
	result          *Analysis
	implementations []rustImplementation
}

type rustImplementation struct {
	moduleID string
	target   string
	trait    string
	members  []Member
}

func rustLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info: Language{
			ID: "rust", Extensions: parserLanguageExtensions("rust"),
			FocusedStructure: true, FocusedFlow: true,
		},
		acceptsSource: func(path string) bool { return codeparser.LanguageFor(path) == "rust" },
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareRustModules(result, sources)
			return &rustAnalysis{result: result}
		},
		nearestProjectRoot: nearestRustRoot,
		sourceScope: func(source Source) (string, error) {
			return absolutePath(source.Path), nil
		},
		normalizeType:     normalizeRustType,
		generateStructure: generateTypeScriptClass,
		generateFlow:      generateTypeScriptFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasTypeScriptCallPath(analysis, source, target) || hasTypeScriptOrderedPath(analysis, source, target)
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
	prepareJVMModules(analysis, sources, "rust", nearestRustRoot)
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
	graph, _ := codeparser.CachedNavigationGraphFromDocument(document, source.Path)
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
	for _, implementation := range analysis.implementations {
		declaration := analysis.rustDeclaration(implementation.moduleID, implementation.target)
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

func (analysis *rustAnalysis) rustDeclaration(moduleID, name string) *Declaration {
	declaration := analysis.result.TSDeclarations[moduleID+":"+name]
	if declaration == nil || declaration.Language != "rust" {
		return nil
	}
	return declaration
}

type rustSourceAnalyzer struct {
	analysis *rustAnalysis
	source   Source
	moduleID string
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
	implementation := rustImplementation{moduleID: analyzer.moduleID, target: target, trait: rustTypeName(node.ChildByFieldName("trait"))}
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
	if analyzer.analysis.result.TSDeclarations[key] != nil {
		analyzer.analysis.result.duplicateErrors = append(analyzer.analysis.result.duplicateErrors, "duplicate Rust type "+declaration.Name+" in "+analyzer.source.Path)
		return
	}
	analyzer.analysis.result.TSDeclarations[key] = declaration
	if analyzer.analysis.result.TSSymbolIndex[key] == nil {
		analyzer.analysis.result.TSSymbolIndex[key] = &Symbol{
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
