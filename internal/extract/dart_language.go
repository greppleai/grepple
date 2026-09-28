package extract

import (
	"strings"

	codeparser "github.com/greppleai/grepple/internal/parser"
)

type dartAnalysis struct{ result *Analysis }

func dartLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info:      Language{ID: "dart", Extensions: parserLanguageExtensions("dart"), FocusedStructure: true, FocusedFlow: true},
		flowIndex: moduleFocusedFlowIndex{}, classIndex: moduleFocusedClassIndex{},
		acceptsSource: func(path string) bool { return codeparser.NewParser().LanguageFor(path) == "dart" },
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareModulePaths(result, sources, "dart", nearestDartRoot)
			return &dartAnalysis{result: result}
		},
		nearestProjectRoot: nearestDartRoot,
		sourceScope:        func(source Source) (string, error) { return absolutePath(source.Path), nil },
		normalizeType:      normalizeDartType,
		generateStructure:  generateModuleClass, generateFlow: generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func nearestDartRoot(directory string) string {
	return nearestMarkedRoot(directory, []string{"pubspec.yaml"})
}

func normalizeDartType(value string) string {
	return normalizeMermaidGeneric(strings.TrimSpace(value))
}

func (analysis *dartAnalysis) Analyze(source Source) error {
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
	addModuleNavigationSymbols(analysis.result, graph, "dart", moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}

func (analysis *dartAnalysis) Finalize() error {
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

func (analysis *dartAnalysis) collectType(node codeparser.ViewNode, source Source, moduleID string) {
	kind := dartFocusedTypeKind(node.Kind())
	if kind == "" {
		return
	}
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return // Unnamed extensions do not have an unambiguous type identity.
	}
	key := moduleID + ":" + name
	if analysis.result.ModuleDeclarations[key] != nil {
		analysis.result.duplicateErrors = append(analysis.result.duplicateErrors, "duplicate dart type "+name+" in "+source.Path)
		return
	}
	stablePath := analysis.result.SourcePaths[absolutePath(source.Path)]
	location := syntaxLocation(source.Path, node)
	location.Path = stablePath
	declaration := &Declaration{Name: name, Kind: kind, Language: "dart", ModuleID: moduleID, File: stablePath, Location: location, Extends: map[string]bool{}, Implements: map[string]bool{}}
	dartCollectHeritage(node, declaration)
	declaration.Members = dartCollectMembers(node.ChildByFieldName("body"), source.Path, stablePath, moduleID)
	analysis.result.ModuleDeclarations[key] = declaration
	analysis.result.ModuleSymbols[key] = &Symbol{Name: name, Kind: kind, Language: "dart", ModuleID: moduleID, Key: key, Calls: map[string]bool{}, Locations: []Location{syntaxLocation(source.Path, node)}}
}

func dartFocusedTypeKind(kind string) string {
	switch kind {
	case "class_declaration":
		return "class"
	case "enum_declaration":
		return "enum"
	case "mixin_declaration":
		return "mixin"
	case "extension_declaration", "extension_type_declaration":
		return "extension"
	default:
		return ""
	}
}

func dartCollectHeritage(node codeparser.ViewNode, declaration *Declaration) {
	if superclass := node.ChildByFieldName("superclass"); superclass.Valid() {
		if parent := directChildByKind(superclass, "type"); parent.Valid() {
			declaration.Extends[normalizeDartType(parent.Text())] = true
		}
	}
	if interfaces := node.ChildByFieldName("interfaces"); interfaces.Valid() {
		for _, candidate := range interfaces.NamedChildren() {
			if candidate.Kind() == "type" && !strings.HasPrefix(candidate.Text(), "<") {
				declaration.Implements[normalizeDartType(candidate.Text())] = true
			}
		}
	}
}

func dartCollectMembers(body codeparser.ViewNode, sourcePath, stablePath, moduleID string) []Member {
	var members []Member
	for _, wrapper := range body.NamedChildren() {
		if wrapper.Kind() != "class_member" {
			continue
		}
		for _, member := range wrapper.NamedChildren() {
			switch member.Kind() {
			case "method_declaration":
				if method, ok := dartMethodMember(member, sourcePath, stablePath, moduleID); ok {
					members = append(members, method)
				}
			case "declaration":
				members = append(members, dartFieldMembers(member, sourcePath, stablePath, moduleID)...)
			}
		}
	}
	return members
}
func dartMethodMember(node codeparser.ViewNode, sourcePath, stablePath, moduleID string) (Member, bool) {
	signature := node.ChildByFieldName("signature")
	if signature.Kind() == "method_signature" {
		for _, child := range signature.NamedChildren() {
			switch child.Kind() {
			case "function_signature", "getter_signature", "setter_signature":
				signature = child
			}
		}
	}
	if !signature.Valid() {
		return Member{}, false
	}
	name := signature.ChildByFieldName("name").Text()
	if name == "" {
		return Member{}, false
	}
	visibility := "public"
	if strings.HasPrefix(name, "_") {
		visibility = "private"
	}
	parameters := []string{}
	for _, parameter := range signature.ChildByFieldName("parameters").NamedChildren() {
		for _, child := range parameter.NamedChildren() {
			if child.Kind() == "type" {
				parameters = append(parameters, normalizeDartType(child.Text()))
				break
			}
		}
	}
	location := syntaxLocation(sourcePath, node)
	location.Path = stablePath
	return Member{Kind: "method", Name: name, Visibility: visibility, Type: normalizeDartType(signature.ChildByFieldName("return_type").Text()), Parameters: parameters, Language: "dart", ModuleID: moduleID, File: stablePath, Location: location}, true
}

func dartFieldMembers(node codeparser.ViewNode, sourcePath, stablePath, moduleID string) []Member {
	typeName := "dynamic"
	for _, child := range node.NamedChildren() {
		if child.Kind() == "type" {
			typeName = normalizeDartType(child.Text())
		}
	}
	var fields []Member
	for _, child := range node.NamedChildren() {
		if child.Kind() != "initialized_identifier_list" {
			continue
		}
		for _, identifier := range child.NamedChildren() {
			name := identifier.ChildByFieldName("name").Text()
			if name == "" {
				continue
			}
			visibility := "public"
			if strings.HasPrefix(name, "_") {
				visibility = "private"
			}
			location := syntaxLocation(sourcePath, identifier)
			location.Path = stablePath
			fields = append(fields, Member{Kind: "property", Name: name, Type: typeName, Visibility: visibility, Language: "dart", ModuleID: moduleID, File: stablePath, Location: location})
		}
	}
	return fields
}
