package parser

import "strings"

func phpNavigationExports(root *syntaxNode, _ string, language, path string) []NavigationExport {
	var exports []NavigationExport
	var add func(*syntaxNode, string)
	add = func(node *syntaxNode, namespace string) {
		switch node.Kind() {
		case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration", "function_definition":
			if name := node.ChildByFieldName("name"); name != nil && name.Text() != "" {
				exports = append(exports, NavigationExport{Name: name.Text(), LocalName: name.Text(), ImportPath: namespace, Language: language, Path: path, Line: node.StartLine()})
			}
		}
	}
	namespace := ""
	for _, node := range root.NamedChildren() {
		if node.Kind() == "namespace_definition" {
			namespace = ""
			if name := node.ChildByFieldName("name"); name != nil {
				namespace = strings.ReplaceAll(name.Text(), `\`, ".")
			}
			if body := node.ChildByFieldName("body"); body != nil {
				for _, child := range body.NamedChildren() {
					add(child, namespace)
				}
				namespace = ""
			}
			continue
		}
		add(node, namespace)
	}
	return exports
}
