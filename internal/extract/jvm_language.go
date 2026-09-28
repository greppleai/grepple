package extract

import (
	"path/filepath"
	"regexp"
	"strings"

	codeparser "github.com/greppleai/grepple/internal/parser"
)

type jvmAnalysis struct {
	result   *Analysis
	language string
}

func javaLanguageDefinition() *languageDefinition {
	return jvmLanguageDefinition("java", nearestJVMRoot)
}

func kotlinLanguageDefinition() *languageDefinition {
	return jvmLanguageDefinition("kotlin", nearestJVMRoot)
}

func cSharpLanguageDefinition() *languageDefinition {
	return jvmLanguageDefinition("csharp", nearestCSharpRoot)
}

func jvmLanguageDefinition(language string, projectRoot func(string) string) *languageDefinition {
	return &languageDefinition{
		info:          Language{ID: language, Extensions: parserLanguageExtensions(language), FocusedStructure: true, FocusedFlow: true},
		flowIndex:     moduleFocusedFlowIndex{},
		classIndex:    moduleFocusedClassIndex{},
		acceptsSource: func(path string) bool { return codeparser.NewParser().LanguageFor(path) == language },
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareModulePaths(result, sources, language, projectRoot)
			return &jvmAnalysis{result: result, language: language}
		},
		nearestProjectRoot: projectRoot,
		sourceScope: func(source Source) (string, error) {
			return absolutePath(source.Path), nil
		},
		normalizeType:     func(value string) string { return normalizeJVMType(value, language) },
		generateStructure: generateModuleClass,
		generateFlow:      generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func normalizeJVMType(value, language string) string {
	value = normalizeTypeScript(value)
	aliases := map[string]string{}
	if language == "kotlin" {
		aliases = map[string]string{"Int": "number", "Long": "number", "Short": "number", "Byte": "number", "Float": "number", "Double": "number", "Unit": "void"}
	} else {
		aliases = map[string]string{"long": "number", "short": "number", "byte": "number", "char": "number", "Long": "number", "Short": "number", "Byte": "number", "Character": "number"}
	}
	for source, target := range aliases {
		value = regexp.MustCompile(`\b`+source+`\b`).ReplaceAllString(value, target)
	}
	return value
}

func nearestCSharpRoot(directory string) string {
	return nearestMarkedRoot(directory, []string{"*.sln", "*.csproj", "*.fsproj"})
}

func nearestMarkedRoot(directory string, patterns []string) string {
	for current := directory; ; current = filepath.Dir(current) {
		for _, pattern := range patterns {
			matches, _ := filepath.Glob(filepath.Join(current, pattern))
			if len(matches) > 0 {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func nearestJVMRoot(directory string) string {
	markers := []string{"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}
	for current := directory; ; current = filepath.Dir(current) {
		for _, marker := range markers {
			if fileExists(filepath.Join(current, marker)) {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func prepareModulePaths(analysis *Analysis, sources []Source, language string, projectRoot func(string) string) {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		if codeparser.NewParser().LanguageFor(source.Path) == language {
			paths = append(paths, absolutePath(source.Path))
		}
	}
	fallbackRoot := commonDirectory(paths)
	for _, sourcePath := range paths {
		root := projectRoot(filepath.Dir(sourcePath))
		if root == "" {
			root = fallbackRoot
		}
		addModulePath(analysis, sourcePath, stableRelativePath(root, sourcePath))
	}
}

func addModulePath(analysis *Analysis, sourcePath, stable string) {
	analysis.ModulePaths[sourcePath] = stable
	if old, ok := analysis.ModuleIndex[stable]; ok && old != sourcePath {
		analysis.ModuleIndex[stable] = ""
	} else {
		analysis.ModuleIndex[stable] = sourcePath
	}
}

func (analysis *jvmAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	analyzer := jvmSourceAnalyzer{result: analysis.result, source: source, moduleID: absolutePath(source.Path), language: analysis.language}
	if err := document.Read(func(view codeparser.DocumentView) error {
		for _, node := range view.Root().NamedChildren() {
			analyzer.analyzeType(node, node)
		}
		return nil
	}); err != nil {
		return err
	}
	graph := codeparser.NewParser().NavigationGraph(document, source.Path)
	addModuleNavigationSymbols(analysis.result, graph, analysis.language, analyzer.moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}

func (analysis *jvmAnalysis) Finalize() error {
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

type jvmSourceAnalyzer struct {
	result   *Analysis
	source   Source
	moduleID string
	language string
}

func (analyzer *jvmSourceAnalyzer) analyzeType(node, envelope codeparser.ViewNode) {
	switch analyzer.language {
	case "java":
		if isJavaTypeNode(node.Kind()) {
			analyzer.analyzeJavaType(node, envelope)
		}
	case "kotlin":
		if node.Kind() == "class_declaration" || node.Kind() == "object_declaration" {
			analyzer.analyzeKotlinType(node, envelope)
		}
	case "csharp":
		if isCSharpTypeNode(node.Kind()) {
			analyzer.analyzeCSharpType(node, envelope)
		}
	}
}

func isJavaTypeNode(kind string) bool {
	switch kind {
	case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
		return true
	default:
		return false
	}
}

func (analyzer *jvmSourceAnalyzer) analyzeCSharpType(node, envelope codeparser.ViewNode) {
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return
	}
	kind := "class"
	if node.Kind() == "interface_declaration" {
		kind = "interface"
	}
	declaration := analyzer.newDeclaration(name, kind, envelope)
	collectCSharpHeritage(node, declaration)
	analyzer.collectCSharpMembers(node, declaration)
	analyzer.storeDeclaration(declaration, envelope)
}

func collectCSharpHeritage(node codeparser.ViewNode, declaration *Declaration) {
	bases := node.ChildByFieldName("bases")
	if !bases.Valid() {
		bases = directChildByKind(node, "base_list")
	}
	names := []string{}
	for _, child := range bases.NamedChildren() {
		name := strings.TrimSpace(strings.TrimPrefix(child.Text(), ":"))
		if name != "" {
			names = append(names, name)
		}
	}
	for index, name := range names {
		if declaration.Kind == "interface" || index == 0 {
			declaration.Extends[name] = true
		} else {
			declaration.Implements[name] = true
		}
	}
}

func (analyzer *jvmSourceAnalyzer) collectCSharpMembers(node codeparser.ViewNode, declaration *Declaration) {
	body := node.ChildByFieldName("body")
	fallback := "private"
	if declaration.Kind == "interface" {
		fallback = "public"
	}
	for _, child := range body.NamedChildren() {
		switch child.Kind() {
		case "method_declaration", "constructor_declaration":
			declaration.Members = appendJVMMember(declaration.Members, analyzer.cSharpMethodMember(child, fallback))
		case "property_declaration":
			member := analyzer.jvmProperty(child.ChildByFieldName("name").Text(), normalizeJVMType(child.ChildByFieldName("type").Text(), "csharp"), child, hasJVMModifier(child, "static"))
			member.Visibility = jvmVisibility(child, fallback)
			declaration.Members = appendJVMMember(declaration.Members, member)
		case "field_declaration":
			for _, member := range analyzer.cSharpFieldMembers(child, fallback) {
				declaration.Members = appendJVMMember(declaration.Members, member)
			}
		case "enum_member_declaration":
			member := analyzer.jvmProperty(child.ChildByFieldName("name").Text(), declaration.Name, child, true)
			member.Visibility = "public"
			declaration.Members = appendJVMMember(declaration.Members, member)
		default:
			analyzer.analyzeType(child, child)
		}
	}
}

func (analyzer *jvmSourceAnalyzer) cSharpMethodMember(node codeparser.ViewNode, fallback string) Member {
	returnType := node.ChildByFieldName("type").Text()
	if returnType == "" {
		returnType = node.ChildByFieldName("returns").Text()
	}
	return Member{
		Kind: "method", Name: node.ChildByFieldName("name").Text(), Visibility: jvmVisibility(node, fallback),
		Type: normalizeJVMType(returnType, "csharp"), Language: analyzer.language,
		ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Static: hasJVMModifier(node, "static"),
		Parameters: javaParameterTypes(node.ChildByFieldName("parameters")), Location: analyzer.location(node),
	}
}

func (analyzer *jvmSourceAnalyzer) cSharpFieldMembers(node codeparser.ViewNode, fallback string) []Member {
	typeName := normalizeJVMType(node.ChildByFieldName("type").Text(), "csharp")
	if typeName == "" {
		typeName = normalizeJVMType(firstDescendantText(node, "predefined_type", "identifier", "generic_name"), "csharp")
	}
	result := []Member{}
	node.WalkNamed(func(child codeparser.ViewNode) {
		if child.Kind() != "variable_declarator" {
			return
		}
		member := analyzer.jvmProperty(child.ChildByFieldName("name").Text(), typeName, node, hasJVMModifier(node, "static"))
		member.Visibility = jvmVisibility(node, fallback)
		result = append(result, member)
	})
	return result
}

func isCSharpTypeNode(kind string) bool {
	switch kind {
	case "class_declaration", "interface_declaration", "struct_declaration", "record_declaration", "enum_declaration":
		return true
	default:
		return false
	}
}

func (analyzer *jvmSourceAnalyzer) newDeclaration(name, kind string, node codeparser.ViewNode) *Declaration {
	return &Declaration{
		Name: name, Kind: kind, Language: analyzer.language, ModuleID: analyzer.moduleID,
		File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Location: analyzer.location(node),
		Extends: map[string]bool{}, Implements: map[string]bool{},
	}
}

func (analyzer *jvmSourceAnalyzer) storeDeclaration(declaration *Declaration, envelope codeparser.ViewNode) {
	key := analyzer.moduleID + ":" + declaration.Name
	if analyzer.result.ModuleDeclarations[key] != nil {
		analyzer.result.duplicateErrors = append(analyzer.result.duplicateErrors, "duplicate "+analyzer.language+" type "+declaration.Name+" in "+analyzer.source.Path)
		return
	}
	analyzer.result.ModuleDeclarations[key] = declaration
	analyzer.addTypeSymbol(declaration.Name, envelope)
}

func (analyzer *jvmSourceAnalyzer) analyzeJavaType(node, envelope codeparser.ViewNode) {
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return
	}
	kind := "class"
	if node.Kind() == "interface_declaration" {
		kind = "interface"
	}
	declaration := analyzer.newDeclaration(name, kind, envelope)
	collectJavaHeritage(node, declaration)
	analyzer.collectJavaMembers(node, declaration)
	analyzer.storeDeclaration(declaration, envelope)
}

func collectJavaHeritage(node codeparser.ViewNode, declaration *Declaration) {
	if superclass := node.ChildByFieldName("superclass"); superclass.Valid() {
		if name := firstContainedTypeText(superclass); name != "" {
			declaration.Extends[name] = true
		}
	}
	if interfaces := node.ChildByFieldName("interfaces"); interfaces.Valid() {
		for _, name := range containedTypeList(interfaces) {
			declaration.Implements[name] = true
		}
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() != "extends_interfaces" {
			continue
		}
		for _, name := range containedTypeList(child) {
			declaration.Extends[name] = true
		}
	}
}

func firstContainedTypeText(node codeparser.ViewNode) string {
	children := node.NamedChildren()
	if len(children) == 0 {
		return strings.TrimSpace(node.Text())
	}
	return strings.TrimSpace(children[0].Text())
}

func containedTypeList(node codeparser.ViewNode) []string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "type_list" {
			result := make([]string, 0, child.NamedChildCount())
			for _, item := range child.NamedChildren() {
				result = append(result, strings.TrimSpace(item.Text()))
			}
			return result
		}
	}
	if name := firstContainedTypeText(node); name != "" {
		return []string{name}
	}
	return nil
}

func (analyzer *jvmSourceAnalyzer) collectJavaMembers(node codeparser.ViewNode, declaration *Declaration) {
	body := node.ChildByFieldName("body")
	if parameters := node.ChildByFieldName("parameters"); parameters.Valid() {
		for _, parameter := range parameters.NamedChildren() {
			if member, ok := analyzer.javaRecordMember(parameter); ok {
				declaration.Members = appendJVMMember(declaration.Members, member)
			}
		}
	}
	for _, child := range body.NamedChildren() {
		switch child.Kind() {
		case "method_declaration", "constructor_declaration":
			declaration.Members = appendJVMMember(declaration.Members, analyzer.javaMethodMember(child, javaMemberVisibility(declaration)))
		case "field_declaration":
			for _, member := range analyzer.javaFieldMembers(child, javaMemberVisibility(declaration), declaration.Kind == "interface") {
				declaration.Members = appendJVMMember(declaration.Members, member)
			}
		case "enum_constant":
			name := child.ChildByFieldName("name").Text()
			if name == "" {
				name = firstDescendantText(child, "identifier")
			}
			member := analyzer.jvmProperty(name, declaration.Name, child, true)
			member.Visibility = "public"
			declaration.Members = appendJVMMember(declaration.Members, member)
		default:
			analyzer.analyzeType(child, child)
		}
	}
}

func javaMemberVisibility(declaration *Declaration) string {
	if declaration.Kind == "interface" {
		return "public"
	}
	return "package"
}

func (analyzer *jvmSourceAnalyzer) javaMethodMember(node codeparser.ViewNode, fallbackVisibility string) Member {
	name := node.ChildByFieldName("name").Text()
	kind := "method"
	return Member{
		Kind: kind, Name: name, Visibility: jvmVisibility(node, fallbackVisibility), Type: normalizeJVMType(node.ChildByFieldName("type").Text(), "java"),
		Language: analyzer.language, ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Static: hasJVMModifier(node, "static"),
		Parameters: javaParameterTypes(node.ChildByFieldName("parameters")), Location: analyzer.location(node),
	}
}

func javaParameterTypes(parameters codeparser.ViewNode) []string {
	result := []string{}
	for _, parameter := range parameters.NamedChildren() {
		typeName := parameter.ChildByFieldName("type").Text()
		if typeName == "" {
			typeName = firstDescendantText(parameter, "type_identifier", "integral_type", "floating_point_type", "boolean_type", "void_type")
		}
		if typeName == "" {
			typeName = "Object"
		}
		result = append(result, normalizeJVMType(typeName, "java"))
	}
	return result
}

func (analyzer *jvmSourceAnalyzer) javaFieldMembers(node codeparser.ViewNode, fallbackVisibility string, forceStatic bool) []Member {
	typeName := normalizeJVMType(node.ChildByFieldName("type").Text(), "java")
	result := []Member{}
	for _, child := range node.NamedChildren() {
		if child.Kind() != "variable_declarator" {
			continue
		}
		member := analyzer.jvmProperty(child.ChildByFieldName("name").Text(), typeName, node, forceStatic || hasJVMModifier(node, "static"))
		member.Visibility = jvmVisibility(node, fallbackVisibility)
		result = append(result, member)
	}
	return result
}

func (analyzer *jvmSourceAnalyzer) javaRecordMember(node codeparser.ViewNode) (Member, bool) {
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return Member{}, false
	}
	typeName := node.ChildByFieldName("type").Text()
	return analyzer.jvmProperty(name, normalizeJVMType(typeName, "java"), node, false), true
}

func (analyzer *jvmSourceAnalyzer) analyzeKotlinType(node, envelope codeparser.ViewNode) {
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return
	}
	kind := "class"
	if kotlinInterfaceDeclaration(node) {
		kind = "interface"
	}
	declaration := analyzer.newDeclaration(name, kind, envelope)
	collectKotlinHeritage(node, declaration)
	analyzer.collectKotlinConstructorProperties(node, declaration)
	analyzer.collectKotlinMembers(node, declaration)
	analyzer.storeDeclaration(declaration, envelope)
}
func kotlinInterfaceDeclaration(node codeparser.ViewNode) bool {
	header := node.Text()
	if body := strings.Index(header, "{"); body >= 0 {
		header = header[:body]
	}
	return containsWord(header, "interface")
}

func collectKotlinHeritage(node codeparser.ViewNode, declaration *Declaration) {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "delegation_specifiers" {
			continue
		}
		for _, specifier := range child.NamedChildren() {
			name := firstDescendantText(specifier, "user_type", "nullable_type", "simple_identifier", "type_identifier")
			if name == "" {
				continue
			}
			if declaration.Kind == "class" && descendantHasKind(specifier, "constructor_invocation") {
				declaration.Extends[name] = true
			} else {
				declaration.Implements[name] = true
			}
		}
	}
}

func (analyzer *jvmSourceAnalyzer) collectKotlinConstructorProperties(node codeparser.ViewNode, declaration *Declaration) {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "primary_constructor" {
			continue
		}
		child.WalkNamed(func(parameter codeparser.ViewNode) {
			if parameter.Kind() != "class_parameter" || !containsWord(parameter.Text(), "val") && !containsWord(parameter.Text(), "var") {
				return
			}
			name := firstDescendantText(parameter, "simple_identifier", "identifier")
			typeName := kotlinTypeFromContainer(parameter)
			member := analyzer.jvmProperty(name, typeName, parameter, false)
			member.Visibility = jvmVisibility(parameter, "public")
			declaration.Members = appendJVMMember(declaration.Members, member)
		})
	}
}

func (analyzer *jvmSourceAnalyzer) collectKotlinMembers(node codeparser.ViewNode, declaration *Declaration) {
	body := directChildByKind(node, "class_body", "enum_class_body")
	static := node.Kind() == "object_declaration"
	for _, child := range body.NamedChildren() {
		switch child.Kind() {
		case "function_declaration":
			member := analyzer.kotlinMethodMember(child)
			member.Static = static
			declaration.Members = appendJVMMember(declaration.Members, member)
		case "property_declaration":
			if member, ok := analyzer.kotlinPropertyMember(child); ok {
				member.Static = static
				declaration.Members = appendJVMMember(declaration.Members, member)
			}
		case "class_declaration", "object_declaration":
			analyzer.analyzeType(child, child)
		}
	}
}

func (analyzer *jvmSourceAnalyzer) kotlinMethodMember(node codeparser.ViewNode) Member {
	name := node.ChildByFieldName("name").Text()
	return Member{
		Kind: "method", Name: name, Visibility: jvmVisibility(node, "public"), Type: kotlinFunctionReturnType(node),
		Language: analyzer.language, ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Static: false,
		Parameters: kotlinParameterTypes(node), Location: analyzer.location(node),
	}
}

func kotlinFunctionReturnType(node codeparser.ViewNode) string {
	seenParameters := false
	for _, child := range node.NamedChildren() {
		if child.Kind() == "function_value_parameters" {
			seenParameters = true
			continue
		}
		if seenParameters && isKotlinTypeNode(child) {
			return normalizeJVMType(child.Text(), "kotlin")
		}
	}
	return ""
}

func kotlinParameterTypes(node codeparser.ViewNode) []string {
	parameters := directChildByKind(node, "function_value_parameters")
	result := []string{}
	for _, parameter := range parameters.NamedChildren() {
		typeName := kotlinTypeFromContainer(parameter)
		if typeName == "" {
			typeName = "Any"
		}
		result = append(result, typeName)
	}
	return result
}

func (analyzer *jvmSourceAnalyzer) kotlinPropertyMember(node codeparser.ViewNode) (Member, bool) {
	variable := firstDescendantByKind(node, "variable_declaration")
	name := firstDescendantText(variable, "simple_identifier", "identifier")
	if name == "" {
		return Member{}, false
	}
	member := analyzer.jvmProperty(name, kotlinTypeFromContainer(variable), node, false)
	member.Visibility = jvmVisibility(node, "public")
	return member, true
}

func kotlinTypeFromContainer(node codeparser.ViewNode) string {
	identifierSeen := false
	for _, child := range node.NamedChildren() {
		if child.Kind() == "simple_identifier" || child.Kind() == "identifier" {
			identifierSeen = true
			continue
		}
		if identifierSeen && isKotlinTypeNode(child) {
			return normalizeJVMType(child.Text(), "kotlin")
		}
		if value := kotlinTypeFromContainer(child); value != "" {
			return value
		}
	}
	return ""
}

func isKotlinTypeNode(node codeparser.ViewNode) bool {
	switch node.Kind() {
	case "user_type", "nullable_type", "function_type", "parenthesized_type", "type_identifier", "dynamic_type":
		return true
	default:
		return false
	}
}

func (analyzer *jvmSourceAnalyzer) jvmProperty(name, typeName string, node codeparser.ViewNode, static bool) Member {
	if typeName == "" {
		typeName = "Any"
	}
	return Member{
		Kind: "property", Name: name, Visibility: jvmVisibility(node, "package"), Type: normalizeJVMType(typeName, analyzer.language),
		Language: analyzer.language, ModuleID: analyzer.moduleID, File: analyzer.stablePath(), Static: static, Location: analyzer.location(node),
	}
}

func appendJVMMember(members []Member, addition Member) []Member {
	if addition.Name == "" {
		return members
	}
	for _, member := range members {
		if member.Name == addition.Name && member.Kind == addition.Kind && strings.Join(member.Parameters, "\x00") == strings.Join(addition.Parameters, "\x00") {
			return members
		}
	}
	return append(members, addition)
}

func jvmVisibility(node codeparser.ViewNode, fallback string) string {
	modifiers := jvmModifiers(node)
	switch {
	case containsWord(modifiers, "private"):
		return "private"
	case containsWord(modifiers, "protected"):
		return "protected"
	case containsWord(modifiers, "internal"):
		return "package"
	case containsWord(modifiers, "public"):
		return "public"
	default:
		return fallback
	}
}

func hasJVMModifier(node codeparser.ViewNode, modifier string) bool {
	return containsWord(jvmModifiers(node), modifier)
}

func jvmModifiers(node codeparser.ViewNode) string {
	if modifiers := directChildByKind(node, "modifiers"); modifiers.Valid() {
		return modifiers.Text()
	}
	var modifiers []string
	for _, child := range node.NamedChildren() {
		if child.Kind() == "modifier" {
			modifiers = append(modifiers, child.Text())
		}
	}
	return strings.Join(modifiers, " ")
}

func containsWord(text, word string) bool {
	for _, field := range strings.FieldsFunc(text, func(character rune) bool {
		return !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '_')
	}) {
		if field == word {
			return true
		}
	}
	return false
}

func directChildByKind(node codeparser.ViewNode, kinds ...string) codeparser.ViewNode {
	for _, child := range node.NamedChildren() {
		for _, kind := range kinds {
			if child.Kind() == kind {
				return child
			}
		}
	}
	return codeparser.ViewNode{}
}

func firstDescendantByKind(node codeparser.ViewNode, kinds ...string) codeparser.ViewNode {
	for _, kind := range kinds {
		if node.Kind() == kind {
			return node
		}
	}
	for _, child := range node.NamedChildren() {
		if found := firstDescendantByKind(child, kinds...); found.Valid() {
			return found
		}
	}
	return codeparser.ViewNode{}
}

func firstDescendantText(node codeparser.ViewNode, kinds ...string) string {
	return strings.TrimSpace(firstDescendantByKind(node, kinds...).Text())
}

func descendantHasKind(node codeparser.ViewNode, kind string) bool {
	return firstDescendantByKind(node, kind).Valid()
}

func (analyzer *jvmSourceAnalyzer) addTypeSymbol(name string, node codeparser.ViewNode) {
	key := analyzer.moduleID + ":" + name
	analyzer.result.ModuleSymbols[key] = &Symbol{
		Name: name, Kind: "class", Language: analyzer.language, ModuleID: analyzer.moduleID, Key: key,
		Calls: map[string]bool{}, Locations: []Location{syntaxLocation(analyzer.source.Path, node)},
	}
}

func (analyzer *jvmSourceAnalyzer) stablePath() string {
	return analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]
}

func (analyzer *jvmSourceAnalyzer) location(node codeparser.ViewNode) Location {
	location := syntaxLocation(analyzer.source.Path, node)
	location.Path = analyzer.stablePath()
	return location
}
