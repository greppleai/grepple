package parser

import "strings"

func ecmaNavigationAdapter(rules *structureRules, fieldFacts, entrypointFacts bool) navigationAdapter {
	adapter := &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression", "new_expression"), extraContainerTypes: newStringSet("interface_declaration"),
		fieldContainerTypes: newStringSet("class_declaration", "interface_declaration"), selfBindingName: "this", selfBindingFromContainer: true,
		visibility: typeScriptAdapterVisibility, sourceFacts: typeScriptNavigationSourceFacts, exports: typeScriptNavigationExports,
		typeReferenceFacts: true, fieldFacts: fieldFacts, memberAccessFacts: true,
		callableReturnBinding: func(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "return_type", "type")
		},
	}
	if entrypointFacts {
		adapter.entrypoint = ecmaNavigationEntrypoint
	}
	return adapter
}

func ecmaNavigationEntrypoint(context navigationEntrypointContext) string {
	if context.node.Kind() != "function_declaration" || context.name == "" || strings.HasSuffix(strings.ToLower(context.path), ".mts") {
		return ""
	}
	root := ecmaTopLevelRoot(context.node)
	if root == nil || ecmaTopLevelFunctionCount(root, context.name) != 1 {
		return ""
	}
	for _, statement := range root.NamedChildren() {
		if statement.Kind() == "if_statement" && ecmaCommonJSGuardCalls(statement, context.name) {
			return "process"
		}
	}
	return ""
}

func ecmaTopLevelRoot(node *syntaxNode) *syntaxNode {
	parent := node.Parent()
	if parent != nil && parent.Kind() == "export_statement" {
		parent = parent.Parent()
	}
	if parent != nil && parent.Kind() == "program" {
		return parent
	}
	return nil
}

func ecmaTopLevelFunctionCount(root *syntaxNode, name string) int {
	count := 0
	for _, statement := range root.NamedChildren() {
		candidate := statement
		if statement.Kind() == "export_statement" {
			for _, child := range statement.NamedChildren() {
				if child.Kind() == "function_declaration" {
					candidate = child
					break
				}
			}
		}
		if candidate.Kind() == "function_declaration" && navigationFieldText(candidate, "name", "") == name {
			count++
		}
	}
	return count
}

func ecmaCommonJSGuardCalls(statement *syntaxNode, name string) bool {
	if !ecmaCommonJSMainCondition(statement.ChildByFieldName("condition")) {
		return false
	}
	consequence := statement.ChildByFieldName("consequence")
	if consequence == nil {
		return false
	}
	if consequence.Kind() == "expression_statement" {
		return ecmaDirectCallStatement(consequence, name)
	}
	if consequence.Kind() != "statement_block" {
		return false
	}
	for _, child := range consequence.NamedChildren() {
		if ecmaDirectCallStatement(child, name) {
			return true
		}
	}
	return false
}

func ecmaDirectCallStatement(statement *syntaxNode, name string) bool {
	if statement.Kind() != "expression_statement" {
		return false
	}
	expressions := statement.NamedChildren()
	if len(expressions) != 1 || expressions[0].Kind() != "call_expression" {
		return false
	}
	function := expressions[0].ChildByFieldName("function")
	return function != nil && function.Kind() == "identifier" && function.Text() == name
}

func ecmaCommonJSMainCondition(condition *syntaxNode) bool {
	for condition != nil && condition.Kind() == "parenthesized_expression" {
		children := condition.NamedChildren()
		if len(children) != 1 {
			return false
		}
		condition = children[0]
	}
	if condition == nil || condition.Kind() != "binary_expression" {
		return false
	}
	left := condition.ChildByFieldName("left")
	right := condition.ChildByFieldName("right")
	if left == nil || right == nil || !ecmaStrictEqualityOperator(condition) {
		return false
	}
	return ecmaCommonJSMainOperands(left, right) || ecmaCommonJSMainOperands(right, left)
}

func ecmaStrictEqualityOperator(condition *syntaxNode) bool {
	if operator := condition.ChildByFieldName("operator"); operator != nil {
		return operator.Text() == "==="
	}
	for _, child := range condition.Children() {
		if child.Kind() == "===" {
			return true
		}
	}
	return false
}

func ecmaCommonJSMainOperands(requireMain, module *syntaxNode) bool {
	if module.Kind() != "identifier" || module.Text() != "module" || requireMain.Kind() != "member_expression" {
		return false
	}
	object := requireMain.ChildByFieldName("object")
	property := requireMain.ChildByFieldName("property")
	return object != nil && property != nil && object.Kind() == "identifier" && object.Text() == "require" && property.Text() == "main"
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
