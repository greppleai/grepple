package extract

import (
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

type phpAnalysis struct{ result *Analysis }

func phpLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info:      Language{ID: "php", Extensions: parserLanguageExtensions("php"), FocusedStructure: true, FocusedFlow: true},
		flowIndex: moduleFocusedFlowIndex{}, classIndex: moduleFocusedClassIndex{},
		acceptsSource: func(path string) bool { return codeparser.LanguageFor(path) == "php" },
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareJVMModules(result, sources, "php", func(string) string { return "" })
			return &phpAnalysis{result: result}
		},
		nearestProjectRoot: func(string) string { return "" },
		sourceScope:        func(source Source) (string, error) { return absolutePath(source.Path), nil },
		normalizeType: func(value string) string {
			return normalizeMermaidGeneric(strings.ReplaceAll(strings.TrimSpace(value), `\`, "."))
		},
		generateStructure: generateModuleClass, generateFlow: generateModuleFlowchart,
		validFlowEdge: func(a *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(a, source, target) || hasModuleOrderedPath(a, source, target)
		},
	}
}

func (analysis *phpAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	moduleID := absolutePath(source.Path)
	path := analysis.result.SourcePaths[moduleID]
	if err := document.Read(func(view codeparser.DocumentView) error {
		var visit func([]codeparser.ViewNode)
		visit = func(nodes []codeparser.ViewNode) {
			for _, node := range nodes {
				switch node.Kind() {
				case "namespace_definition":
					if body := node.ChildByFieldName("body"); body.Valid() {
						visit(body.NamedChildren())
					}
				case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
					name := node.ChildByFieldName("name").Text()
					if name == "" {
						continue
					}
					declaration := &Declaration{Name: name, Kind: "class", Language: "php", ModuleID: moduleID, File: path, Location: syntaxLocation(path, node), Extends: map[string]bool{}, Implements: map[string]bool{}}
					for _, clause := range node.NamedChildren() {
						target := declaration.Extends
						if clause.Kind() == "class_interface_clause" {
							target = declaration.Implements
						} else if clause.Kind() != "base_clause" {
							continue
						}
						for _, typeName := range clause.NamedChildren() {
							target[strings.ReplaceAll(typeName.Text(), `\`, ".")] = true
						}
					}
					if body := node.ChildByFieldName("body"); body.Valid() {
						for _, member := range body.NamedChildren() {
							if member.Kind() != "method_declaration" && member.Kind() != "property_declaration" {
								continue
							}
							visibility := "public"
							static := false
							for _, child := range member.NamedChildren() {
								switch child.Kind() {
								case "visibility_modifier":
									visibility = child.Text()
								case "static_modifier":
									static = true
								}
							}
							if member.Kind() == "method_declaration" {
								method := Member{Name: member.ChildByFieldName("name").Text(), Kind: "method", Visibility: visibility, Static: static, Type: member.ChildByFieldName("return_type").Text(), Language: "php", ModuleID: moduleID, File: path, Location: syntaxLocation(path, member)}
								for _, parameter := range member.ChildByFieldName("parameters").NamedChildren() {
									if typ := parameter.ChildByFieldName("type"); typ.Valid() {
										method.Parameters = append(method.Parameters, typ.Text())
									} else {
										method.Parameters = append(method.Parameters, "mixed")
									}
								}
								declaration.Members = append(declaration.Members, method)
							} else {
								for _, element := range member.NamedChildren() {
									if element.Kind() == "property_element" {
										declaration.Members = append(declaration.Members, Member{Name: strings.TrimPrefix(element.ChildByFieldName("name").Text(), "$"), Kind: "property", Visibility: visibility, Static: static, Type: member.ChildByFieldName("type").Text(), Language: "php", ModuleID: moduleID, File: path, Location: syntaxLocation(path, element)})
									}
								}
							}
						}
					}
					key := moduleID + ":" + name
					analysis.result.ModuleDeclarations[key] = declaration
					analysis.result.ModuleSymbols[key] = &Symbol{Name: name, Kind: "class", Language: "php", ModuleID: moduleID, Key: key, Calls: map[string]bool{}, Locations: []Location{declaration.Location}}
				}
			}
		}
		visit(view.Root().NamedChildren())
		return nil
	}); err != nil {
		return err
	}
	graph, _ := codeparser.CachedNavigationGraphFromDocument(document, source.Path)
	addModuleNavigationSymbols(analysis.result, graph, "php", moduleID, source.Path)
	analysis.result.Navigation.Merge(graph)
	return nil
}
func (analysis *phpAnalysis) Finalize() error { finalizeTypeScriptIndexes(analysis.result); return nil }
