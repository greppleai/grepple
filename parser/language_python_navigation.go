package parser

import "strings"

func pythonNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call"), parameterTypes: newStringSet("typed_parameter", "typed_default_parameter"), sourceFacts: pythonNavigationSourceFacts, typeReferenceFacts: true, memberAccessFacts: true, visibility: func(_ *syntaxNode, name, _ string) NavigationVisibility {
		return pythonNavigationVisibility(navigationTerminal(name))
	}}
}

func pythonNavigationSourceFacts(root *syntaxNode, _ string, _ *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	for _, node := range root.NamedChildren() {
		switch node.Kind() {
		case "import_statement":
			addPythonNavigationImports(imports, node)
		case "import_from_statement":
			addPythonNavigationFromImports(imports, node)
		}
	}
	return imports, packageName, fields
}

func addPythonNavigationImports(imports map[string]navigationImport, node *syntaxNode) {
	for index, child := range node.NamedChildren() {
		if node.FieldNameForNamedChild(uint32(index)) != "name" {
			continue
		}
		importPath, alias := pythonNavigationImportName(child)
		if importPath == "" {
			continue
		}
		if alias == "" {
			alias = strings.Split(importPath, ".")[0]
		}
		addScopedNavigationImport(imports, alias, importPath, "*", node.StartLine())
	}
}

func addPythonNavigationFromImports(imports map[string]navigationImport, node *syntaxNode) {
	module := node.ChildByFieldName("module_name")
	if module == nil || strings.TrimSpace(module.Text()) == "" {
		return
	}
	importPath := strings.TrimSpace(module.Text())
	for index, child := range node.NamedChildren() {
		if child.ID() == module.ID() {
			continue
		}
		field := node.FieldNameForNamedChild(uint32(index))
		if field != "name" && child.Kind() != "wildcard_import" {
			continue
		}
		imported, alias := pythonNavigationImportName(child)
		if child.Kind() == "wildcard_import" {
			imported, alias = "*", "*"
		} else if alias == "" {
			alias = navigationTerminal(imported)
		}
		addScopedNavigationImport(imports, alias, importPath, imported, node.StartLine())
	}
}

func pythonNavigationImportName(node *syntaxNode) (string, string) {
	if node == nil {
		return "", ""
	}
	if node.Kind() != "aliased_import" {
		return strings.TrimSpace(node.Text()), ""
	}
	name := navigationFirstField(node, "name")
	alias := navigationFirstField(node, "alias")
	if name == nil || alias == nil {
		return "", ""
	}
	return strings.TrimSpace(name.Text()), strings.TrimSpace(alias.Text())
}
