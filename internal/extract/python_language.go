package extract

import (
	"path/filepath"
	"strings"

	codeparser "github.com/greppleai/grepple/internal/parser"
)

type pythonAnalysis struct {
	result *Analysis
}

func pythonLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info: Language{
			ID: "python", Extensions: parserLanguageExtensions("python"),
			FocusedStructure: true, FocusedFlow: true,
		},
		flowIndex:     moduleFocusedFlowIndex{},
		classIndex:    moduleFocusedClassIndex{},
		acceptsSource: isPythonSourceFile,
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			preparePythonModules(result, sources)
			return &pythonAnalysis{result: result}
		},
		nearestProjectRoot: nearestPythonRoot,
		sourceScope: func(source Source) (string, error) {
			return absolutePath(source.Path), nil
		},
		normalizeType:     normalizePythonType,
		generateStructure: generateModuleClass,
		generateFlow:      generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func isPythonSourceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".pyi", ".pyw":
		return true
	default:
		return false
	}
}

func nearestPythonRoot(directory string) string {
	for current := directory; ; current = filepath.Dir(current) {
		if fileExists(filepath.Join(current, "pyproject.toml")) || fileExists(filepath.Join(current, "setup.py")) || fileExists(filepath.Join(current, "setup.cfg")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func normalizePythonType(value string) string {
	return normalizeType(strings.Trim(strings.TrimSpace(value), "'\""))
}

func preparePythonModules(analysis *Analysis, sources []Source) {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		if codeparser.NewParser().LanguageFor(source.Path) == "python" {
			paths = append(paths, absolutePath(source.Path))
		}
	}
	fallbackRoot := commonDirectory(paths)
	for _, sourcePath := range paths {
		root := nearestPythonRoot(filepath.Dir(sourcePath))
		if root == "" {
			root = fallbackRoot
		}
		stable := stableRelativePath(root, sourcePath)
		analysis.ModulePaths[sourcePath] = stable
		if old, ok := analysis.ModuleIndex[stable]; ok && old != sourcePath {
			analysis.ModuleIndex[stable] = ""
		} else {
			analysis.ModuleIndex[stable] = sourcePath
		}
	}
}

func (analysis *pythonAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	analyzer := pythonSourceAnalyzer{
		result: analysis.result, source: source, moduleID: absolutePath(source.Path),
	}
	if err := document.Read(func(view codeparser.DocumentView) error {
		for _, node := range view.Root().NamedChildren() {
			analyzer.analyzeDefinition(node, node)
		}
		return nil
	}); err != nil {
		return err
	}
	graph := codeparser.NewParser().NavigationGraph(document, source.Path)
	addModuleNavigationSymbols(analysis.result, graph, "python", analyzer.moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}

func (analysis *pythonAnalysis) Finalize() error {
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

type pythonSourceAnalyzer struct {
	result   *Analysis
	source   Source
	moduleID string
}

func (analyzer *pythonSourceAnalyzer) analyzeDefinition(node, envelope codeparser.ViewNode) {
	definition := pythonUnwrappedDefinition(node)
	if !definition.Valid() {
		return
	}
	switch definition.Kind() {
	case "class_definition":
		analyzer.analyzeClass(definition, envelope)
	}
}

func pythonUnwrappedDefinition(node codeparser.ViewNode) codeparser.ViewNode {
	if node.Kind() != "decorated_definition" {
		return node
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() == "class_definition" || child.Kind() == "function_definition" {
			return child
		}
	}
	return codeparser.ViewNode{}
}

func (analyzer *pythonSourceAnalyzer) analyzeClass(node, envelope codeparser.ViewNode) {
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return
	}
	declaration := &Declaration{
		Name: name, Kind: "class", Language: "python", ModuleID: analyzer.moduleID,
		File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Location: analyzer.location(envelope),
		Extends: map[string]bool{}, Implements: map[string]bool{},
	}
	collectPythonSuperclasses(node.ChildByFieldName("superclasses"), declaration)
	analyzer.collectPythonClassBody(node.ChildByFieldName("body"), declaration)
	key := analyzer.moduleID + ":" + name
	if analyzer.result.ModuleDeclarations[key] != nil {
		analyzer.result.duplicateErrors = append(analyzer.result.duplicateErrors, "duplicate Python class "+name+" in "+analyzer.source.Path)
		return
	}
	analyzer.result.ModuleDeclarations[key] = declaration
	analyzer.addSymbol(name, "class", "", envelope)
}

func collectPythonSuperclasses(superclasses codeparser.ViewNode, declaration *Declaration) {
	for _, superclass := range superclasses.NamedChildren() {
		if superclass.Kind() != "keyword_argument" {
			declaration.Extends[strings.TrimSpace(superclass.Text())] = true
		}
	}
}

func (analyzer *pythonSourceAnalyzer) collectPythonClassBody(body codeparser.ViewNode, declaration *Declaration) {
	for _, child := range body.NamedChildren() {
		definition := pythonUnwrappedDefinition(child)
		switch definition.Kind() {
		case "function_definition":
			analyzer.addPythonMethod(definition, child, declaration)
		case "class_definition":
			analyzer.analyzeClass(definition, child)
		default:
			if member, ok := analyzer.pythonAssignmentMember(child, false); ok {
				declaration.Members = appendPythonMember(declaration.Members, member)
			}
		}
	}
}

func (analyzer *pythonSourceAnalyzer) addPythonMethod(node, envelope codeparser.ViewNode, declaration *Declaration) {
	member := analyzer.pythonFunctionMember(node, envelope)
	declaration.Members = appendPythonMember(declaration.Members, member)
	analyzer.collectPythonInstanceFields(node, declaration)
}

func (analyzer *pythonSourceAnalyzer) pythonFunctionMember(node, envelope codeparser.ViewNode) Member {
	name := node.ChildByFieldName("name").Text()
	kind := "method"
	if strings.Contains(envelope.Text(), "@property") {
		kind = "property"
	}
	return Member{
		Kind: kind, Name: name, Visibility: pythonVisibility(name), Type: normalizePythonType(node.ChildByFieldName("return_type").Text()),
		Language: "python", ModuleID: analyzer.moduleID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)],
		Static: strings.Contains(envelope.Text(), "@staticmethod") || strings.Contains(envelope.Text(), "@classmethod"),
		Async:  strings.HasPrefix(strings.TrimSpace(node.Text()), "async "), Parameters: pythonParameterTypes(node), Location: analyzer.location(envelope),
	}
}

func pythonParameterTypes(node codeparser.ViewNode) []string {
	parameters := node.ChildByFieldName("parameters")
	result := []string{}
	for _, parameter := range parameters.NamedChildren() {
		name := pythonFirstIdentifier(parameter)
		if name == "self" || name == "cls" {
			continue
		}
		typeName := pythonNestedFieldText(parameter, "type")
		if typeName == "" {
			typeName = "Any"
		}
		result = append(result, normalizePythonType(typeName))
	}
	return result
}

func pythonFirstIdentifier(node codeparser.ViewNode) string {
	if node.Kind() == "identifier" {
		return node.Text()
	}
	for _, child := range node.NamedChildren() {
		if name := pythonFirstIdentifier(child); name != "" {
			return name
		}
	}
	return ""
}

func pythonNestedFieldText(node codeparser.ViewNode, field string) string {
	if child := node.ChildByFieldName(field); child.Valid() {
		return child.Text()
	}
	for _, child := range node.NamedChildren() {
		if value := pythonNestedFieldText(child, field); value != "" {
			return value
		}
	}
	return ""
}

func (analyzer *pythonSourceAnalyzer) collectPythonInstanceFields(function codeparser.ViewNode, declaration *Declaration) {
	function.ChildByFieldName("body").WalkNamed(func(node codeparser.ViewNode) {
		if member, ok := analyzer.pythonAssignmentMember(node, true); ok {
			declaration.Members = appendPythonMember(declaration.Members, member)
		}
	})
}

func (analyzer *pythonSourceAnalyzer) pythonAssignmentMember(node codeparser.ViewNode, instance bool) (Member, bool) {
	if node.Kind() == "expression_statement" && node.NamedChildCount() == 1 {
		node = node.NamedChild(0)
	}
	if node.Kind() != "assignment" {
		return Member{}, false
	}
	left := node.ChildByFieldName("left")
	name := ""
	if !instance && left.Kind() == "identifier" {
		name = left.Text()
	}
	if instance && left.Kind() == "attribute" && left.ChildByFieldName("object").Text() == "self" {
		name = left.ChildByFieldName("attribute").Text()
	}
	if name == "" {
		return Member{}, false
	}
	typeName := normalizePythonType(node.ChildByFieldName("type").Text())
	if typeName == "" {
		typeName = "Any"
	}
	return Member{
		Kind: "property", Name: name, Visibility: pythonVisibility(name), Type: typeName, Language: "python",
		ModuleID: analyzer.moduleID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Static: !instance, Location: analyzer.location(node),
	}, true
}

func appendPythonMember(members []Member, addition Member) []Member {
	if addition.Name == "" {
		return members
	}
	for index := range members {
		if members[index].Name == addition.Name && members[index].Kind == addition.Kind {
			if members[index].Type == "Any" && addition.Type != "Any" {
				members[index] = addition
			}
			return members
		}
	}
	return append(members, addition)
}

func pythonVisibility(name string) string {
	if strings.HasPrefix(name, "__") && !strings.HasSuffix(name, "__") {
		return "private"
	}
	if strings.HasPrefix(name, "_") && !strings.HasPrefix(name, "__") {
		return "protected"
	}
	return "public"
}

func (analyzer *pythonSourceAnalyzer) addSymbol(name, kind, owner string, node codeparser.ViewNode) {
	key := analyzer.moduleID + ":" + name
	symbol := analyzer.result.ModuleSymbols[key]
	if symbol == nil {
		symbol = &Symbol{Name: name, Kind: kind, Language: "python", ModuleID: analyzer.moduleID, Key: key, Owner: owner, Calls: map[string]bool{}}
		analyzer.result.ModuleSymbols[key] = symbol
	}
	symbol.Locations = append(symbol.Locations, syntaxLocation(analyzer.source.Path, node))
}

func (analyzer *pythonSourceAnalyzer) location(node codeparser.ViewNode) Location {
	location := syntaxLocation(analyzer.source.Path, node)
	location.Path = analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]
	return location
}
