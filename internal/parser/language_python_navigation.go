package parser

import "strings"

func pythonNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call"), parameterTypes: newStringSet("typed_parameter", "typed_default_parameter"),
		fieldDeclarationTypes: newStringSet("assignment"), fieldContainerTypes: rules.classDeclarationTypes,
		sourceFacts: pythonNavigationSourceFacts, typeReferenceFacts: true, fieldFacts: true, memberAccessFacts: true,
		fieldNames: pythonNavigationFieldNames, entrypoint: pythonNavigationEntrypoint,
		visibility: func(_ *syntaxNode, name, _ string) NavigationVisibility {
			return pythonNavigationVisibility(navigationTerminal(name))
		},
	}
}

func pythonNavigationEntrypoint(context navigationEntrypointContext) string {
	node := context.node
	if node.Kind() != "function_definition" || context.name == "" || context.container != "" || strings.HasSuffix(strings.ToLower(context.path), ".pyi") || navigationHeaderHasWord(navigationDeclarationHeader(node, context.content), "async") {
		return ""
	}
	root := node.Parent()
	if root == nil || root.Kind() != "module" || pythonTopLevelFunctionCount(root, context.name) != 1 {
		return ""
	}
	for _, statement := range root.NamedChildren() {
		if statement.Kind() == "if_statement" && pythonMainModuleGuardCalls(statement, context.name) {
			return "process"
		}
	}
	return ""
}

func pythonTopLevelFunctionCount(root *syntaxNode, name string) int {
	count := 0
	for _, statement := range root.NamedChildren() {
		if statement.Kind() == "function_definition" && navigationFieldText(statement, "name", "") == name {
			count++
		}
	}
	return count
}

func pythonMainModuleGuardCalls(statement *syntaxNode, name string) bool {
	condition := statement.ChildByFieldName("condition")
	consequence := statement.ChildByFieldName("consequence")
	if !pythonMainModuleCondition(condition) || consequence == nil || consequence.Kind() != "block" {
		return false
	}
	for _, child := range consequence.NamedChildren() {
		if child.Kind() != "expression_statement" {
			continue
		}
		expressions := child.NamedChildren()
		if len(expressions) != 1 || expressions[0].Kind() != "call" {
			continue
		}
		function := expressions[0].ChildByFieldName("function")
		if function != nil && function.Kind() == "identifier" && function.Text() == name {
			return true
		}
	}
	return false
}

func pythonMainModuleCondition(condition *syntaxNode) bool {
	if condition == nil || condition.Kind() != "comparison_operator" {
		return false
	}
	children := condition.Children()
	if len(children) != 3 || children[1].Kind() != "==" {
		return false
	}
	return pythonMainModuleOperands(children[0], children[2]) || pythonMainModuleOperands(children[2], children[0])
}

func pythonMainModuleOperands(name, value *syntaxNode) bool {
	if name.Kind() != "identifier" || name.Text() != "__name__" || value.Kind() != "string" {
		return false
	}
	return value.Text() == `"__main__"` || value.Text() == `'__main__'`
}

func pythonNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	for _, node := range root.NamedChildren() {
		switch node.Kind() {
		case "import_statement":
			addPythonNavigationImports(imports, node)
		case "import_from_statement":
			addPythonNavigationFromImports(imports, node)
		}
	}
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, packageName, fields
}

func pythonNavigationFieldNames(node, typeNode *syntaxNode, content string) []navigationFieldName {
	statement := node.Parent()
	if statement == nil || statement.Kind() != "expression_statement" {
		return nil
	}
	body := statement.Parent()
	if body == nil || body.Kind() != "block" {
		return nil
	}
	owner := body.Parent()
	if owner == nil || owner.Kind() != "class_definition" {
		return nil
	}
	names := defaultNavigationParameterNames(node, typeNode, content)
	result := make([]navigationFieldName, 0, len(names))
	for _, name := range names {
		result = append(result, navigationFieldName{name: name})
	}
	return result
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
