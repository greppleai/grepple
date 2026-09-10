package parser

import (
	"path"
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type navigationImport struct {
	path     string
	imported string
}

type navigationBinding struct {
	typeName   string
	importPath string
}

func navigationSourceFacts(root *sitter.Node, content, language string) (map[string]navigationImport, string) {
	imports := make(map[string]navigationImport)
	packageName := ""
	walkNodes(root, func(node *sitter.Node) {
		switch {
		case language == "go" && node.Kind() == "package_clause":
			packageName = navigationPackageName(node, content)
		case language == "go" && node.Kind() == "import_spec":
			addGoNavigationImport(imports, node, content)
		case (language == "typescript" || language == "tsx" || language == "javascript") && node.Kind() == "import_statement":
			addTypeScriptNavigationImports(imports, node, content)
		}
	})
	return imports, packageName
}

func navigationPackageName(node *sitter.Node, content string) string {
	for _, child := range namedChildren(node) {
		if child.Kind() == "package_identifier" || child.Kind() == "identifier" {
			return nodeText(child, content)
		}
	}
	return ""
}

func addGoNavigationImport(imports map[string]navigationImport, node *sitter.Node, content string) {
	pathNode := node.ChildByFieldName("path")
	if pathNode == nil {
		return
	}
	importPath := unquoteNavigationPath(nodeText(pathNode, content))
	if importPath == "" {
		return
	}
	alias := path.Base(importPath)
	if name := node.ChildByFieldName("name"); name != nil {
		alias = nodeText(name, content)
	}
	if alias == "" || alias == "_" || alias == "." {
		return
	}
	imports[alias] = navigationImport{path: importPath, imported: "*"}
}

func addTypeScriptNavigationImports(imports map[string]navigationImport, node *sitter.Node, content string) {
	source := node.ChildByFieldName("source")
	if source == nil {
		return
	}
	importPath := unquoteNavigationPath(nodeText(source, content))
	walkNodes(node, func(current *sitter.Node) {
		switch current.Kind() {
		case "import_specifier":
			imported := navigationFieldText(current, "name", content)
			local := navigationFieldText(current, "alias", content)
			if local == "" {
				local = imported
			}
			if local != "" {
				imports[local] = navigationImport{path: importPath, imported: imported}
			}
		case "namespace_import":
			if local := navigationFirstIdentifier(current, content); local != "" {
				imports[local] = navigationImport{path: importPath, imported: "*"}
			}
		}
	})
}

func navigationFieldText(node *sitter.Node, field, content string) string {
	child := node.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return nodeText(child, content)
}

func navigationFirstIdentifier(node *sitter.Node, content string) string {
	for _, child := range namedChildren(node) {
		if child.Kind() == "identifier" {
			return nodeText(child, content)
		}
	}
	return ""
}

func unquoteNavigationPath(value string) string {
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, "'\"`")
}

func navigationCallableBindings(node *sitter.Node, content, language, container string, imports map[string]navigationImport) map[string]navigationBinding {
	bindings := make(map[string]navigationBinding)
	if (language == "typescript" || language == "tsx" || language == "javascript") && container != "" {
		bindings["this"] = navigationBinding{typeName: container}
	}
	for _, field := range []string{"receiver", "parameters"} {
		root := node.ChildByFieldName(field)
		if root == nil {
			continue
		}
		walkNodes(root, func(current *sitter.Node) {
			addNavigationParameterBinding(bindings, current, content, imports)
		})
	}
	return bindings
}

func addNavigationParameterBinding(bindings map[string]navigationBinding, node *sitter.Node, content string, imports map[string]navigationImport) {
	switch node.Kind() {
	case "parameter_declaration", "variadic_parameter_declaration", "required_parameter", "optional_parameter":
	default:
		return
	}
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	binding := navigationBindingForType(nodeText(typeNode, content), imports)
	if binding.typeName == "" {
		return
	}
	for _, name := range navigationParameterNames(node, typeNode, content) {
		bindings[name] = binding
	}
}

func navigationParameterNames(node, typeNode *sitter.Node, content string) []string {
	if name := node.ChildByFieldName("name"); name != nil {
		return []string{nodeText(name, content)}
	}
	if pattern := node.ChildByFieldName("pattern"); pattern != nil {
		return []string{nodeText(pattern, content)}
	}
	var names []string
	for _, child := range namedChildren(node) {
		if child.Id() != typeNode.Id() && (child.Kind() == "identifier" || child.Kind() == "field_identifier") {
			names = append(names, nodeText(child, content))
		}
	}
	return names
}

func navigationTypeName(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), ":"))
	value = strings.TrimLeft(value, "*[]&? ")
	if index := strings.IndexAny(value, "<[{|( "); index >= 0 {
		value = value[:index]
	}
	if index := strings.LastIndex(value, "."); index >= 0 {
		value = value[index+1:]
	}
	return strings.Trim(value, "*[]&? ")
}

func navigationBindingForType(value string, imports map[string]navigationImport) navigationBinding {
	binding := navigationBinding{typeName: navigationTypeName(value)}
	qualifier := navigationTypeQualifier(value)
	if qualifier == "" {
		qualifier = binding.typeName
	}
	if imported, ok := imports[qualifier]; ok {
		binding.importPath = imported.path
	}
	return binding
}

func navigationTypeQualifier(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), ":"))
	value = strings.TrimLeft(value, "*[]&? ")
	if index := strings.IndexAny(value, "<[{|( "); index >= 0 {
		value = value[:index]
	}
	if index := strings.LastIndex(value, "."); index > 0 {
		return value[:index]
	}
	return ""
}

func applyNavigationCallContext(call *NavigationCall, imports map[string]navigationImport, bindings map[string]navigationBinding) {
	if call.Qualifier != "" {
		applyQualifiedNavigationCallContext(call, imports, bindings)
		return
	}
	if imported, ok := imports[call.Name]; ok && imported.imported != "*" {
		call.ImportPath = imported.path
		call.ResolvedName = imported.imported
	}
}

func applyQualifiedNavigationCallContext(call *NavigationCall, imports map[string]navigationImport, bindings map[string]navigationBinding) {
	if imported, ok := imports[call.Qualifier]; ok {
		call.ImportPath = imported.path
		call.ResolvedName = call.Name
		return
	}
	compact := strings.Join(strings.Fields(call.Display), "")
	if strings.Count(compact, ".") != 1 {
		return
	}
	binding := bindings[call.Qualifier]
	call.ReceiverType = binding.typeName
	call.ImportPath = binding.importPath
}
