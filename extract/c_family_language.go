package extract

import (
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

type cFamilyAnalysis struct {
	result   *Analysis
	language string
	cpp      bool
}

func cLanguageDefinition() *languageDefinition {
	return cFamilyLanguageDefinition("c", false)
}

func cppLanguageDefinition() *languageDefinition {
	return cFamilyLanguageDefinition("cpp", true)
}

func cFamilyLanguageDefinition(language string, cpp bool) *languageDefinition {
	noProjectRoot := func(string) string { return "" }
	return &languageDefinition{
		info:          Language{ID: language, Extensions: parserLanguageExtensions(language), FocusedStructure: true, FocusedFlow: true},
		acceptsSource: func(path string) bool { return codeparser.LanguageFor(path) == language },
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareJVMModules(result, sources, language, noProjectRoot)
			return &cFamilyAnalysis{result: result, language: language, cpp: cpp}
		},
		nearestProjectRoot: noProjectRoot,
		sourceScope: func(source Source) (string, error) {
			return absolutePath(source.Path), nil
		},
		normalizeType:     normalizeCFamilyType,
		generateStructure: generateTypeScriptClass,
		generateFlow:      generateTypeScriptFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasTypeScriptCallPath(analysis, source, target) || hasTypeScriptOrderedPath(analysis, source, target)
		},
	}
}

func (analysis *cFamilyAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	analyzer := cFamilySourceAnalyzer{
		analysis: analysis,
		source:   source,
		moduleID: absolutePath(source.Path),
		language: analysis.language,
		cpp:      analysis.cpp,
	}
	if err := document.Read(func(view codeparser.DocumentView) error {
		analyzer.analyzeItems(view.Root().NamedChildren())
		return nil
	}); err != nil {
		return err
	}
	graph, _ := codeparser.CachedNavigationGraphFromDocument(document, source.Path)
	addCFamilyNavigationSymbols(analysis.result, graph, analysis.language, analyzer.moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}

func (analysis *cFamilyAnalysis) Finalize() error {
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

type cFamilySourceAnalyzer struct {
	analysis *cFamilyAnalysis
	source   Source
	moduleID string
	language string
	scope    string
	cpp      bool
}

func (analyzer *cFamilySourceAnalyzer) analyzeItems(nodes []codeparser.ViewNode) {
	for _, node := range nodes {
		analyzer.analyzeItem(node)
	}
}

func (analyzer *cFamilySourceAnalyzer) analyzeItem(node codeparser.ViewNode) {
	switch node.Kind() {
	case "namespace_definition":
		analyzer.analyzeNamespace(node)
	case "type_definition":
		analyzer.analyzeTypeDefinition(node)
	case "struct_specifier", "union_specifier", "enum_specifier", "class_specifier":
		analyzer.analyzeAggregate(node, "")
	case "declaration":
		analyzer.analyzeDeclarationType(node)
	}
}

func (analyzer *cFamilySourceAnalyzer) analyzeNamespace(node codeparser.ViewNode) {
	if !analyzer.cpp {
		return
	}
	body := node.ChildByFieldName("body")
	if !body.Valid() {
		return
	}
	nested := *analyzer
	nested.scope = joinCFamilyScope(analyzer.scope, node.ChildByFieldName("name").Text())
	nested.analyzeItems(body.NamedChildren())
}

func (analyzer *cFamilySourceAnalyzer) analyzeTypeDefinition(node codeparser.ViewNode) {
	typeNode := node.ChildByFieldName("type")
	if !cFamilyAggregateNode(typeNode.Kind(), analyzer.cpp) {
		return
	}
	alias := cFamilyDeclaratorName(firstCFamilyField(node, "declarator"))
	analyzer.analyzeAggregate(typeNode, alias)
}

func (analyzer *cFamilySourceAnalyzer) analyzeDeclarationType(node codeparser.ViewNode) {
	typeNode := node.ChildByFieldName("type")
	if cFamilyAggregateNode(typeNode.Kind(), analyzer.cpp) {
		analyzer.analyzeAggregate(typeNode, "")
	}
}

func (analyzer *cFamilySourceAnalyzer) analyzeAggregate(node codeparser.ViewNode, alias string) {
	name := strings.TrimSpace(alias)
	if name == "" {
		name = strings.TrimSpace(node.ChildByFieldName("name").Text())
	}
	if name == "" {
		return
	}
	declaration := &Declaration{
		Name: name, Kind: "class", Language: analyzer.language, ModuleID: analyzer.moduleID,
		File: analyzer.stablePath(), Location: analyzer.location(node), Extends: map[string]bool{}, Implements: map[string]bool{},
	}
	if analyzer.cpp {
		for _, base := range cFamilyBaseTypes(node) {
			declaration.Extends[base] = true
		}
	}
	if node.Kind() == "enum_specifier" {
		declaration.Members = analyzer.enumMembers(node, name)
	} else {
		declaration.Members = analyzer.aggregateMembers(node)
	}
	analyzer.storeDeclaration(declaration)
}

func cFamilyBaseTypes(node codeparser.ViewNode) []string {
	bases := []string{}
	for _, child := range node.NamedChildren() {
		if child.Kind() != "base_class_clause" {
			continue
		}
		for _, base := range child.NamedChildren() {
			switch base.Kind() {
			case "type_identifier", "qualified_identifier", "namespace_identifier":
				if name := normalizeCFamilyType(base.Text()); name != "" {
					bases = append(bases, name)
				}
			}
		}
	}
	return bases
}

func cFamilyAggregateNode(kind string, cpp bool) bool {
	if kind == "struct_specifier" || kind == "union_specifier" || kind == "enum_specifier" {
		return true
	}
	return cpp && kind == "class_specifier"
}

func (analyzer *cFamilySourceAnalyzer) aggregateMembers(node codeparser.ViewNode) []Member {
	body := node.ChildByFieldName("body")
	if !body.Valid() {
		return nil
	}
	visibility := cFamilyDefaultVisibility(node.Kind(), analyzer.cpp)
	members := []Member{}
	for _, item := range body.NamedChildren() {
		if item.Kind() == "access_specifier" {
			visibility = strings.TrimSuffix(strings.TrimSpace(item.Text()), ":")
			continue
		}
		members = append(members, analyzer.aggregateItemMembers(item, visibility)...)
	}
	return members
}

func cFamilyDefaultVisibility(kind string, cpp bool) string {
	if cpp && kind == "class_specifier" {
		return "private"
	}
	return "public"
}

func (analyzer *cFamilySourceAnalyzer) aggregateItemMembers(node codeparser.ViewNode, visibility string) []Member {
	switch node.Kind() {
	case "function_definition":
		if member, ok := analyzer.functionMember(node, visibility); ok {
			return []Member{member}
		}
	case "field_declaration":
		if analyzer.cpp {
			if member, ok := analyzer.declaredMethodMember(node, visibility); ok {
				return []Member{member}
			}
		}
		return analyzer.fieldMembers(node, visibility)
	}
	return nil
}

func (analyzer *cFamilySourceAnalyzer) declaredMethodMember(node codeparser.ViewNode, visibility string) (Member, bool) {
	for _, declarator := range cFamilyFieldChildren(node, "declarator") {
		function := cFamilyDescendantByKind(declarator, "function_declarator")
		if function.Valid() && !cFamilyDeclaratorHasPointer(function.ChildByFieldName("declarator")) {
			return analyzer.callableMember(node, function, visibility)
		}
	}
	return Member{}, false
}

func (analyzer *cFamilySourceAnalyzer) functionMember(node codeparser.ViewNode, visibility string) (Member, bool) {
	function := cFamilyDescendantByKind(node.ChildByFieldName("declarator"), "function_declarator")
	if !function.Valid() {
		return Member{}, false
	}
	return analyzer.callableMember(node, function, visibility)
}

func (analyzer *cFamilySourceAnalyzer) callableMember(node, function codeparser.ViewNode, visibility string) (Member, bool) {
	name := cFamilyDeclaratorName(function.ChildByFieldName("declarator"))
	if name == "" {
		return Member{}, false
	}
	return Member{
		Kind: "method", Name: name, Visibility: visibility, Type: normalizeCFamilyType(cFamilyTypeText(node)),
		Parameters: cFamilyParameterTypes(function), Static: cFamilyHasDirectChild(node, "storage_class_specifier", "static"),
		Language: analyzer.language, ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(node),
	}, true
}

func (analyzer *cFamilySourceAnalyzer) fieldMembers(node codeparser.ViewNode, visibility string) []Member {
	members := []Member{}
	for _, declarator := range cFamilyFieldChildren(node, "declarator") {
		name := cFamilyDeclaratorName(declarator)
		if name == "" || cFamilyDescendantByKind(declarator, "function_declarator").Valid() {
			continue
		}
		members = append(members, Member{
			Kind: "property", Name: name, Visibility: visibility, Type: cFamilyDeclaredType(node, declarator, name),
			Static: cFamilyHasDirectChild(node, "storage_class_specifier", "static"), Language: analyzer.language,
			ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(declarator),
		})
	}
	return members
}

func (analyzer *cFamilySourceAnalyzer) enumMembers(node codeparser.ViewNode, owner string) []Member {
	members := []Member{}
	for _, item := range node.ChildByFieldName("body").NamedChildren() {
		if item.Kind() != "enumerator" {
			continue
		}
		name := strings.TrimSpace(item.ChildByFieldName("name").Text())
		if name == "" {
			continue
		}
		members = append(members, Member{
			Kind: "property", Name: name, Type: owner, Visibility: "public", Static: true,
			Language: analyzer.language, ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Location: analyzer.location(item),
		})
	}
	return members
}

func cFamilyParameterTypes(function codeparser.ViewNode) []string {
	parameters := []string{}
	for _, parameter := range function.ChildByFieldName("parameters").NamedChildren() {
		if parameter.Kind() == "variadic_parameter_declaration" {
			parameters = append(parameters, "...")
			continue
		}
		if parameter.Kind() != "parameter_declaration" && parameter.Kind() != "optional_parameter_declaration" {
			continue
		}
		declarator := firstCFamilyField(parameter, "declarator")
		name := cFamilyDeclaratorName(declarator)
		parameters = append(parameters, cFamilyDeclaredType(parameter, declarator, name))
	}
	return parameters
}

func cFamilyDeclaredType(node, declarator codeparser.ViewNode, name string) string {
	value := cFamilyTypeText(node)
	if declarator.Valid() && name != "" {
		suffix := strings.TrimSpace(strings.Replace(declarator.Text(), name, "", 1))
		value += suffix
	}
	return normalizeCFamilyType(value)
}

func cFamilyTypeText(node codeparser.ViewNode) string {
	parts := []string{}
	for _, child := range node.NamedChildren() {
		if child.FieldName() == "type" || child.Kind() == "type_qualifier" {
			parts = append(parts, child.Text())
		}
	}
	return strings.Join(parts, " ")
}

func normalizeCFamilyType(value string) string {
	return normalizeMermaidGeneric(strings.ReplaceAll(strings.TrimSpace(value), "::", "."))
}

func cFamilyDeclaratorName(node codeparser.ViewNode) string {
	if !node.Valid() {
		return ""
	}
	switch node.Kind() {
	case "identifier", "field_identifier", "type_identifier", "namespace_identifier", "destructor_name":
		return strings.TrimSpace(node.Text())
	}
	for _, field := range []string{"declarator", "name"} {
		if name := cFamilyDeclaratorName(node.ChildByFieldName(field)); name != "" {
			return name
		}
	}
	return ""
}

func cFamilyFieldChildren(node codeparser.ViewNode, field string) []codeparser.ViewNode {
	children := []codeparser.ViewNode{}
	for _, child := range node.NamedChildren() {
		if child.FieldName() == field {
			children = append(children, child)
		}
	}
	return children
}

func firstCFamilyField(node codeparser.ViewNode, field string) codeparser.ViewNode {
	children := cFamilyFieldChildren(node, field)
	if len(children) == 0 {
		return codeparser.ViewNode{}
	}
	return children[0]
}

func cFamilyDescendantByKind(node codeparser.ViewNode, kind string) codeparser.ViewNode {
	if !node.Valid() {
		return codeparser.ViewNode{}
	}
	if node.Kind() == kind {
		return node
	}
	for _, child := range node.NamedChildren() {
		if found := cFamilyDescendantByKind(child, kind); found.Valid() {
			return found
		}
	}
	return codeparser.ViewNode{}
}

func cFamilyDeclaratorHasPointer(node codeparser.ViewNode) bool {
	for node.Valid() {
		if node.Kind() == "pointer_declarator" || node.Kind() == "abstract_pointer_declarator" {
			return true
		}
		node = node.ChildByFieldName("declarator")
	}
	return false
}

func cFamilyHasDirectChild(node codeparser.ViewNode, kind, text string) bool {
	for _, child := range node.NamedChildren() {
		if child.Kind() == kind && strings.TrimSpace(child.Text()) == text {
			return true
		}
	}
	return false
}

func joinCFamilyScope(scope, name string) string {
	name = strings.TrimSpace(name)
	if scope == "" {
		return name
	}
	if name == "" {
		return scope
	}
	return scope + "::" + name
}

func (analyzer *cFamilySourceAnalyzer) storeDeclaration(declaration *Declaration) {
	key := analyzer.moduleID + ":" + joinCFamilyScope(analyzer.scope, declaration.Name)
	existing := analyzer.analysis.result.TSDeclarations[key]
	if existing == nil || len(existing.Members) < len(declaration.Members) {
		analyzer.analysis.result.TSDeclarations[key] = declaration
	}
	if analyzer.analysis.result.TSSymbolIndex[key] == nil {
		analyzer.analysis.result.TSSymbolIndex[key] = &Symbol{
			Name: declaration.Name, Kind: "class", Language: analyzer.language, ModuleID: analyzer.moduleID, Key: key,
			Calls: map[string]bool{}, Locations: []Location{declaration.Location},
		}
	}
}

func (analyzer *cFamilySourceAnalyzer) stablePath() string {
	return analyzer.analysis.result.SourcePaths[absolutePath(analyzer.source.Path)]
}

func (analyzer *cFamilySourceAnalyzer) location(node codeparser.ViewNode) Location {
	location := syntaxLocation(analyzer.source.Path, node)
	location.Path = analyzer.stablePath()
	return location
}

func addCFamilyNavigationSymbols(analysis *Analysis, graph codeparser.NavigationGraph, language, moduleID, sourcePath string) {
	for _, declaration := range graph.Declarations {
		base := moduleID + ":" + declaration.Name
		key := cFamilyNavigationSymbolKey(analysis.TSSymbolIndex, base, declaration.ID)
		analysis.TSSymbolIndex[key] = &Symbol{
			Name: declaration.Name, Kind: declaration.Kind, Language: language, ModuleID: moduleID, Key: key,
			Owner: declaration.Container, NavigationID: declaration.ID, Calls: map[string]bool{},
			Locations: []Location{{Path: sourcePath, Line: declaration.Start, EndLine: declaration.End}},
		}
	}
}

func cFamilyNavigationSymbolKey(symbols map[string]*Symbol, base, declarationID string) string {
	if existing := symbols[base]; existing != nil {
		if existing.NavigationID == declarationID {
			return base
		}
		delete(symbols, base)
		existing.Key = base + "\x00" + existing.NavigationID
		symbols[existing.Key] = existing
		return base + "\x00" + declarationID
	}
	prefix := base + "\x00"
	if symbols[prefix+declarationID] != nil {
		return prefix + declarationID
	}
	for key := range symbols {
		if strings.HasPrefix(key, prefix) {
			return prefix + declarationID
		}
	}
	return base
}
