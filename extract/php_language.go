package extract

import (
	"path/filepath"
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

// PHP source files are the module boundary. Composer is a project-root marker,
// not evidence that a namespace maps to a particular file or package.
type phpAnalysis struct{ result *Analysis }

func phpLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info:      Language{ID: "php", Extensions: parserLanguageExtensions("php"), FocusedStructure: true, FocusedFlow: true},
		flowIndex: moduleFocusedFlowIndex{}, classIndex: moduleFocusedClassIndex{},
		acceptsSource: func(path string) bool { return codeparser.LanguageFor(path) == "php" },
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareJVMModules(result, sources, "php", nearestPHPRoot)
			return &phpAnalysis{result: result}
		},
		nearestProjectRoot: nearestPHPRoot,
		sourceScope:        func(source Source) (string, error) { return absolutePath(source.Path), nil },
		normalizeType:      normalizePHPType,
		generateStructure:  generateModuleClass,
		generateFlow:       generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func nearestPHPRoot(directory string) string {
	for current := directory; ; current = filepath.Dir(current) {
		if fileExists(filepath.Join(current, "composer.json")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func normalizePHPType(value string) string {
	return normalizeMermaidGeneric(strings.ReplaceAll(strings.TrimSpace(value), "\\", "."))
}

func (analysis *phpAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	analyzer := phpSourceAnalyzer{sourceAnalyzer: sourceAnalyzer{result: analysis.result, source: source, moduleID: absolutePath(source.Path), language: "php"}}
	if err := document.Read(func(view codeparser.DocumentView) error {
		analyzer.analyzeItems(view.Root().NamedChildren(), "")
		return nil
	}); err != nil {
		return err
	}
	graph, _ := codeparser.CachedNavigationGraphFromDocument(document, source.Path)
	addModuleNavigationSymbols(analysis.result, graph, "php", analyzer.moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}

func (analysis *phpAnalysis) Finalize() error {
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

type phpSourceAnalyzer struct{ sourceAnalyzer }

func (analyzer *phpSourceAnalyzer) analyzeItems(nodes []codeparser.ViewNode, namespace string) {
	for _, node := range nodes {
		switch node.Kind() {
		case "namespace_definition":
			name := strings.TrimPrefix(node.ChildByFieldName("name").Text(), "\\")
			if body := node.ChildByFieldName("body"); body.Valid() {
				analyzer.analyzeItems(body.NamedChildren(), name)
			} else {
				namespace = name
			} // unbraced namespace applies to following siblings
		case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
			analyzer.analyzeType(node, namespace)
		}
	}
}

func (analyzer *phpSourceAnalyzer) analyzeType(node codeparser.ViewNode, namespace string) {
	name := node.ChildByFieldName("name").Text()
	if name == "" {
		return
	} // anonymous classes have no declaration identity
	kind := "class"
	if node.Kind() == "interface_declaration" || node.Kind() == "trait_declaration" {
		kind = "interface"
	}
	declaration := &Declaration{
		Name: name, Kind: kind, Language: "php", Package: namespace, ModuleID: analyzer.moduleID,
		File: analyzer.result.SourcePaths[analyzer.moduleID], Location: analyzer.location(node),
		Extends: map[string]bool{}, Implements: map[string]bool{},
	}
	for _, child := range node.NamedChildren() {
		switch child.Kind() {
		case "base_clause":
			for _, base := range child.NamedChildren() {
				if name := normalizePHPType(base.Text()); name != "" {
					declaration.Extends[name] = true
				}
			}
		case "class_interface_clause":
			for _, iface := range child.NamedChildren() {
				if name := normalizePHPType(iface.Text()); name != "" {
					declaration.Implements[name] = true
				}
			}
		}
	}
	body := node.ChildByFieldName("body")
	for _, member := range body.NamedChildren() {
		switch member.Kind() {
		case "method_declaration":
			declaration.Members = append(declaration.Members, analyzer.method(member))
		case "property_declaration":
			declaration.Members = append(declaration.Members, analyzer.properties(member)...)
		case "enum_case":
			name := member.ChildByFieldName("name").Text()
			if name != "" {
				declaration.Members = append(declaration.Members, Member{Kind: "property", Name: name, Type: name, Visibility: "public", Static: true, Language: "php", ModuleID: analyzer.moduleID, File: declaration.File, Location: analyzer.location(member)})
			}
		case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
			analyzer.analyzeType(member, namespace)
		}
	}
	key := analyzer.moduleID + ":" + name
	if analyzer.result.ModuleDeclarations[key] != nil {
		analyzer.result.duplicateErrors = append(analyzer.result.duplicateErrors, "duplicate php type "+name+" in "+analyzer.source.Path)
		return
	}
	analyzer.result.ModuleDeclarations[key] = declaration
}

func phpModifier(node codeparser.ViewNode, kind string) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == kind {
			return strings.TrimSpace(child.Text())
		}
	}
	return ""
}

func phpVisibility(node codeparser.ViewNode) string {
	switch phpModifier(node, "visibility_modifier") {
	case "private":
		return "private"
	case "protected":
		return "protected"
	default:
		return "public"
	}
}

func (analyzer *phpSourceAnalyzer) method(node codeparser.ViewNode) Member {
	params := []string{}
	for _, param := range node.ChildByFieldName("parameters").NamedChildren() {
		switch param.Kind() {
		case "simple_parameter", "variadic_parameter", "property_promotion_parameter":
			value := normalizePHPType(param.ChildByFieldName("type").Text())
			if value == "" {
				value = "mixed"
			}
			params = append(params, value)
		}
	}
	return Member{Kind: "method", Name: node.ChildByFieldName("name").Text(), Visibility: phpVisibility(node),
		Type: normalizePHPType(node.ChildByFieldName("return_type").Text()), Parameters: params,
		Static: phpModifier(node, "static_modifier") != "", Language: "php", ModuleID: analyzer.moduleID,
		File: analyzer.result.SourcePaths[analyzer.moduleID], Location: analyzer.location(node)}
}

func (analyzer *phpSourceAnalyzer) properties(node codeparser.ViewNode) []Member {
	members := []Member{}
	for _, child := range node.NamedChildren() {
		if child.Kind() != "property_element" {
			continue
		}
		for _, item := range child.NamedChildren() {
			if item.Kind() != "variable_name" {
				continue
			}
			members = append(members, Member{Kind: "property", Name: strings.TrimPrefix(item.Text(), "$"), Type: normalizePHPType(node.ChildByFieldName("type").Text()),
				Visibility: phpVisibility(node), Static: phpModifier(node, "static_modifier") != "",
				Language: "php", ModuleID: analyzer.moduleID, File: analyzer.result.SourcePaths[analyzer.moduleID], Location: analyzer.location(child)})
			break
		}
	}
	return members
}
