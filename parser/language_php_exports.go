package parser

import "strings"

// Only explicit PHP namespace declarations become export identities. Composer
// autoload mappings and dynamic includes are deliberately not inferred.
func phpExports(root *syntaxNode, _ string, language, path string) []NavigationExport {
	var exports []NavigationExport
	namespace := ""
	for _, node := range root.NamedChildren() {
		if node.Kind() == "namespace_definition" {
			namespace = strings.TrimPrefix(navigationFieldText(node, "name", ""), "\\")
			if body := node.ChildByFieldName("body"); body != nil {
				exports = append(exports, phpExportsInScope(body, namespace, language, path)...)
				namespace = ""
			}
			continue
		}
		exports = append(exports, phpExportDeclaration(node, namespace, language, path)...)
	}
	return exports
}

func phpExportsInScope(scope *syntaxNode, namespace, language, path string) []NavigationExport {
	var exports []NavigationExport
	for _, node := range scope.NamedChildren() {
		exports = append(exports, phpExportDeclaration(node, namespace, language, path)...)
	}
	return exports
}

func phpExportDeclaration(node *syntaxNode, namespace, language, path string) []NavigationExport {
	switch node.Kind() {
	case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration", "function_definition":
		name := node.ChildByFieldName("name")
		if name != nil && name.Text() != "" {
			return []NavigationExport{{Name: name.Text(), LocalName: name.Text(), ImportPath: namespace, Language: language, Path: path, Line: node.StartLine()}}
		}
	}
	return nil
}
