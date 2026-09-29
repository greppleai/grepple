package extract

import (
	"strings"

	codeparser "github.com/greppleai/grepple/internal/parser"
)

type swiftAnalysis struct {
	result    *Analysis
	inherited map[string][]string
}

func swiftLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info:          Language{ID: "swift", Extensions: parserLanguageExtensions("swift"), FocusedStructure: true, FocusedFlow: true},
		flowIndex:     moduleFocusedFlowIndex{},
		classIndex:    moduleFocusedClassIndex{},
		acceptsSource: func(path string) bool { return codeparser.NewParser().LanguageFor(path) == "swift" },
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareModulePaths(result, sources, "swift", nearestSwiftRoot)
			return &swiftAnalysis{result: result, inherited: map[string][]string{}}
		},
		nearestProjectRoot: nearestSwiftRoot,
		sourceScope:        func(source Source) (string, error) { return absolutePath(source.Path), nil },
		normalizeType:      func(value string) string { return normalizeMermaidGeneric(strings.TrimSpace(value)) },
		generateStructure:  generateModuleClass,
		generateFlow:       generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func nearestSwiftRoot(directory string) string {
	return nearestMarkedRoot(directory, []string{"Package.swift"})
}

func (analysis *swiftAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	moduleID := absolutePath(source.Path)
	if err := document.Read(func(view codeparser.DocumentView) error {
		for _, node := range view.Root().NamedChildren() {
			analysis.collectType(node, source, moduleID)
		}
		return nil
	}); err != nil {
		return err
	}
	graph := codeparser.NewParser().NavigationGraph(document, source.Path)
	addModuleNavigationSymbols(analysis.result, graph, "swift", moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}

func (analysis *swiftAnalysis) Finalize() error {
	for key, inherited := range analysis.inherited {
		declaration := analysis.result.ModuleDeclarations[key]
		for index, name := range inherited {
			if declaration.Kind != "class" || index > 0 {
				declaration.Implements[name] = true
				continue
			}
			// A Swift class's first inherited name may be a superclass or a
			// protocol. Assert an edge only for a same-file declared target.
			target := analysis.result.ModuleDeclarations[declaration.ModuleID+":"+name]
			if target == nil {
				continue
			}
			if target.Kind == "class" {
				declaration.Extends[name] = true
			} else if target.Kind == "interface" {
				declaration.Implements[name] = true
			}
		}
	}
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

func (analysis *swiftAnalysis) collectType(node codeparser.ViewNode, source Source, moduleID string) {
	kind := swiftFocusedTypeKind(node)
	if kind == "" {
		return
	}
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return
	}
	key := moduleID + ":" + name
	if analysis.result.ModuleDeclarations[key] != nil {
		analysis.result.duplicateErrors = append(analysis.result.duplicateErrors, "duplicate swift type "+name+" in "+source.Path)
		return
	}
	stablePath := analysis.result.SourcePaths[moduleID]
	location := syntaxLocation(source.Path, node)
	location.Path = stablePath
	declaration := &Declaration{Name: name, Kind: kind, Language: "swift", ModuleID: moduleID, File: stablePath, Location: location, Extends: map[string]bool{}, Implements: map[string]bool{}}
	analysis.inherited[key] = swiftInheritedTypes(node)
	declaration.Members = swiftCollectMembers(node.ChildByFieldName("body"), source.Path, stablePath, moduleID)
	analysis.result.ModuleDeclarations[key] = declaration
	analysis.result.ModuleSymbols[key] = &Symbol{Name: name, Kind: kind, Language: "swift", ModuleID: moduleID, Key: key, Calls: map[string]bool{}, Locations: []Location{syntaxLocation(source.Path, node)}}
}

func swiftFocusedTypeKind(node codeparser.ViewNode) string {
	if node.Kind() == "protocol_declaration" {
		return "interface"
	}
	if node.Kind() != "class_declaration" {
		return ""
	}
	switch node.ChildByFieldName("declaration_kind").Text() {
	case "class", "struct", "enum":
		return node.ChildByFieldName("declaration_kind").Text()
	default:
		return "" // Extensions attach to existing types; do not invent another declaration.
	}
}

func swiftInheritedTypes(node codeparser.ViewNode) []string {
	var inherited []string
	for _, child := range node.NamedChildren() {
		if child.Kind() == "inheritance_specifier" {
			if name := child.ChildByFieldName("inherits_from").Text(); name != "" {
				inherited = append(inherited, normalizeMermaidGeneric(name))
			}
		}
	}
	return inherited
}

func swiftCollectMembers(body codeparser.ViewNode, sourcePath, stablePath, moduleID string) []Member {
	var members []Member
	for _, node := range body.NamedChildren() {
		var member Member
		var ok bool
		switch node.Kind() {
		case "function_declaration", "protocol_function_declaration", "init_declaration":
			member, ok = swiftMethodMember(node, sourcePath, stablePath, moduleID)
		case "property_declaration", "protocol_property_declaration":
			member, ok = swiftPropertyMember(node, sourcePath, stablePath, moduleID)
		}
		if ok {
			members = append(members, member)
		}
	}
	return members
}

func swiftMethodMember(node codeparser.ViewNode, sourcePath, stablePath, moduleID string) (Member, bool) {
	name := node.ChildByFieldName("name").Text()
	if node.Kind() == "init_declaration" {
		name = "init"
	}
	if name == "" {
		return Member{}, false
	}
	var parameters []string
	for _, parameter := range node.NamedChildren() {
		if parameter.Kind() == "parameter" {
			if typeName := parameter.ChildByFieldName("type").Text(); typeName != "" {
				parameters = append(parameters, normalizeMermaidGeneric(typeName))
			}
		}
	}
	location := syntaxLocation(sourcePath, node)
	location.Path = stablePath
	return Member{Kind: "method", Name: name, Type: normalizeMermaidGeneric(node.ChildByFieldName("return_type").Text()), Parameters: parameters, Visibility: swiftMemberVisibility(node), Language: "swift", ModuleID: moduleID, File: stablePath, Location: location}, true
}

func swiftPropertyMember(node codeparser.ViewNode, sourcePath, stablePath, moduleID string) (Member, bool) {
	name := node.ChildByFieldName("name").Text()
	if name == "" || strings.ContainsAny(name, ",()") {
		return Member{}, false
	}
	var typeName string
	for _, child := range node.NamedChildren() {
		if child.Kind() == "type_annotation" {
			typeName = child.ChildByFieldName("type").Text()
			if typeName == "" {
				typeName = child.ChildByFieldName("name").Text()
			}
		}
	}
	if typeName == "" {
		return Member{}, false // Inferred properties cannot supply a reliable type.
	}
	location := syntaxLocation(sourcePath, node)
	location.Path = stablePath
	return Member{Kind: "property", Name: name, Type: normalizeMermaidGeneric(typeName), Visibility: swiftMemberVisibility(node), Language: "swift", ModuleID: moduleID, File: stablePath, Location: location}, true
}

func swiftMemberVisibility(node codeparser.ViewNode) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "modifiers" {
			continue
		}
		for _, modifier := range child.NamedChildren() {
			if modifier.Kind() != "visibility_modifier" {
				continue
			}
			switch modifier.Text() {
			case "public", "open":
				return "public"
			case "private", "fileprivate":
				return "private"
			}
		}
	}
	return "package" // Swift internal is module-scoped; Mermaid's ~ is the conservative projection.
}
