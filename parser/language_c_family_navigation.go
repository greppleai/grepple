package parser

import (
	"strconv"
	"strings"
)

func cFamilyNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression"), fieldContainerTypes: rules.classDeclarationTypes, sourceFacts: cFamilyNavigationSourceFacts,
		typeReferenceFacts: true, fieldFacts: true, memberAccessFacts: true,
		declarationName: func(node *syntaxNode, content string, _ *navigationEnvelope) string {
			return cFamilyName(node, content, rules)
		},
		entrypoint: cFamilyNavigationEntrypoint,
	}
}

func cFamilyNavigationEntrypoint(context navigationEntrypointContext) string {
	node, name := context.node, context.name
	if node.Kind() != "function_definition" || name != "main" {
		return ""
	}
	returnType := node.ChildByFieldName("type")
	if returnType == nil || strings.TrimSpace(returnType.Text()) != "int" {
		return ""
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() == "storage_class_specifier" {
			return ""
		}
	}
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "namespace_definition", "class_specifier", "struct_specifier", "union_specifier":
			return ""
		}
	}
	return "process"
}

func cFamilyNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	root.WalkNamed(func(node *syntaxNode) {
		item, ok := cFamilyNavigationInclude(node)
		if ok {
			imports[navigationImportUniqueKey(item)] = item
		}
	})
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, packageName, fields
}

func cFamilyNavigationInclude(node *syntaxNode) (navigationImport, bool) {
	if node.Kind() != "preproc_include" {
		return navigationImport{}, false
	}
	pathNode := node.ChildByFieldName("path")
	if pathNode == nil {
		return navigationImport{}, false
	}
	value := strings.TrimSpace(pathNode.Text())
	kind := ""
	switch pathNode.Kind() {
	case "string_literal":
		decoded, err := strconv.Unquote(value)
		if err != nil || decoded == "" || strings.ContainsRune(decoded, '\x00') {
			return navigationImport{}, false
		}
		value, kind = decoded, "include-quoted"
	case "system_lib_string":
		if len(value) < 3 || value[0] != '<' || value[len(value)-1] != '>' {
			return navigationImport{}, false
		}
		value, kind = strings.TrimSpace(value[1:len(value)-1]), "include-system"
	default:
		return navigationImport{}, false
	}
	if value == "" {
		return navigationImport{}, false
	}
	return navigationImport{alias: "*", path: value, imported: "*", kind: kind, line: node.StartLine()}, true
}
