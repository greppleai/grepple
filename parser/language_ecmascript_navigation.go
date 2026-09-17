package parser

import "strings"

func ecmaNavigationAdapter(rules *structureRules, fieldFacts bool) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression", "new_expression"), extraContainerTypes: newStringSet("interface_declaration"),
		fieldContainerTypes: newStringSet("class_declaration", "interface_declaration"), selfBindingName: "this", selfBindingFromContainer: true,
		visibility: typeScriptAdapterVisibility, sourceFacts: typeScriptNavigationSourceFacts, exports: typeScriptNavigationExports,
		typeReferenceFacts: true, fieldFacts: fieldFacts, memberAccessFacts: true,
		callableReturnBinding: func(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "return_type", "type")
		},
	}
}

func typeScriptNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	root.WalkNamed(func(node *syntaxNode) {
		if node.Kind() == "import_statement" {
			addTypeScriptNavigationImports(imports, node, content)
		}
	})
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	collectTypeScriptNavigationHeritage(root, imports, fields)
	return imports, packageName, fields
}

func collectTypeScriptNavigationHeritage(root *syntaxNode, imports map[string]navigationImport, fields map[string]map[string]navigationBinding) {
	root.WalkNamed(func(node *syntaxNode) {
		addTypeScriptNavigationHeritage(node, imports, fields)
	})
}

func addTypeScriptNavigationHeritage(node *syntaxNode, imports map[string]navigationImport, fields map[string]map[string]navigationBinding) {
	if node.Kind() != "class_declaration" && node.Kind() != "interface_declaration" {
		return
	}
	name := navigationFieldText(node, "name", "")
	if name == "" {
		return
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() != "class_heritage" {
			continue
		}
		for _, clause := range child.NamedChildren() {
			addTypeScriptNavigationHeritageClause(name, clause, imports, fields)
		}
	}
}

func addTypeScriptNavigationHeritageClause(name string, clause *syntaxNode, imports map[string]navigationImport, fields map[string]map[string]navigationBinding) {
	if clause.Kind() != "extends_clause" && clause.Kind() != "implements_clause" {
		return
	}
	for _, typeNode := range clause.NamedChildren() {
		binding := navigationBindingForType(typeNode.Text(), imports)
		if binding.typeName == "" {
			continue
		}
		binding.embedded = true
		binding.line = clause.StartLine()
		if fields[name] == nil {
			fields[name] = make(map[string]navigationBinding)
		}
		fields[name][binding.typeName] = binding
	}
}

func typeScriptAdapterVisibility(node *syntaxNode, name, content string) NavigationVisibility {
	return typeScriptNavigationVisibility(node, navigationTerminal(name), navigationDeclarationHeader(node, content))
}

func addTypeScriptNavigationImports(imports map[string]navigationImport, node *syntaxNode, content string) {
	source := node.ChildByFieldName("source")
	if source == nil {
		return
	}
	importPath := unquoteNavigationPath(source.Text())
	addTypeScriptDefaultNavigationImport(imports, node, importPath)
	node.WalkNamed(func(current *syntaxNode) {
		switch current.Kind() {
		case "import_specifier":
			imported := navigationFieldText(current, "name", content)
			local := navigationFieldText(current, "alias", content)
			if local == "" {
				local = imported
			}
			if local != "" {
				imports[local] = navigationImport{alias: local, path: importPath, imported: imported, line: node.StartLine()}
			}
		case "namespace_import":
			if local := navigationFirstIdentifier(current, content); local != "" {
				imports[local] = navigationImport{alias: local, path: importPath, imported: "*", line: node.StartLine()}
			}
		}
	})
}

func typeScriptNavigationExports(root *syntaxNode, _ string, language, path string) []NavigationExport {
	exports := []NavigationExport{}
	root.WalkNamed(func(node *syntaxNode) {
		exports = append(exports, typeScriptNavigationExportsForNode(node, language, path)...)
	})
	return exports
}

func typeScriptNavigationExportsForNode(node *syntaxNode, language, path string) []NavigationExport {
	if node.Kind() != "export_statement" {
		return nil
	}
	source := node.ChildByFieldName("source")
	importPath := ""
	if source != nil {
		importPath = unquoteNavigationPath(source.Text())
	}
	exports := typeScriptNavigationExportSpecifiers(node, importPath, language, path)
	if len(exports) > 0 {
		return exports
	}
	if source != nil {
		return []NavigationExport{{Name: "*", ImportPath: importPath, ImportedName: "*", Language: language, Path: path, Line: node.StartLine()}}
	}
	if exported, ok := typeScriptDefaultNavigationExport(node, language, path); ok {
		return []NavigationExport{exported}
	}
	return nil
}

func typeScriptNavigationExportSpecifiers(node *syntaxNode, importPath, language, path string) []NavigationExport {
	exports := []NavigationExport{}
	node.WalkNamed(func(current *syntaxNode) {
		if current.Kind() != "export_specifier" {
			return
		}
		local := navigationFieldText(current, "name", "")
		exported := navigationFieldText(current, "alias", "")
		if exported == "" {
			exported = local
		}
		exports = append(exports, NavigationExport{Name: exported, LocalName: local, ImportPath: importPath, ImportedName: local, Language: language, Path: path, Line: current.StartLine()})
	})
	return exports
}

func typeScriptDefaultNavigationExport(node *syntaxNode, language, path string) (NavigationExport, bool) {
	if !strings.HasPrefix(strings.TrimSpace(node.Text()), "export default ") {
		return NavigationExport{}, false
	}
	for _, child := range node.NamedChildren() {
		name := navigationFieldText(child, "name", "")
		if name != "" {
			return NavigationExport{Name: "default", LocalName: name, ImportedName: name, Language: language, Path: path, Line: node.StartLine()}, true
		}
	}
	return NavigationExport{}, false
}

func addTypeScriptDefaultNavigationImport(imports map[string]navigationImport, node *syntaxNode, importPath string) {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "import_clause" {
			continue
		}
		for _, imported := range child.NamedChildren() {
			if imported.Kind() == "identifier" {
				local := imported.Text()
				imports[local] = navigationImport{alias: local, path: importPath, imported: "default", line: node.StartLine()}
			}
		}
		return
	}
}
