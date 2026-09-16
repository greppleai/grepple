package parser

import (
	"path"
	"strconv"
	"strings"
)

func goNavigationMethodName(node *syntaxNode, content string, rules *structureRules) string {
	name := extractNodeName(node, content, rules)
	receiver := strings.TrimSuffix(strings.TrimPrefix(goReceiver(node, content), "("), ").")
	receiver = strings.TrimPrefix(receiver, "*")
	if receiver == "" {
		return name
	}
	return receiver + "." + name
}

func goNavigationAdapter(rules *structureRules) navigationAdapter {
	adapter := &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression"), extraContainerTypes: newStringSet("type_spec"),
		fieldContainerTypes: newStringSet("type_spec"), isCallable: func(node *syntaxNode) bool {
			if rules.functionLikeTypes.contains(node.Kind()) || node.Kind() == "method_elem" || node.Kind() == "method_spec" {
				return true
			}
			callableType := node.ChildByFieldName("type")
			return node.Kind() == "field_declaration" && callableType != nil && callableType.Kind() == "function_type"
		},
		requiresContainer: func(node *syntaxNode) bool {
			return node.Kind() == "method_elem" || node.Kind() == "method_spec" || node.Kind() == "field_declaration"
		},
		declarationName: func(node *syntaxNode, content string, _ *navigationEnvelope) string {
			if node.Kind() == "method_declaration" {
				return goNavigationMethodName(node, content, rules)
			}
			return extractNodeName(node, content, rules)
		},
		declarationKind: func(node *syntaxNode, container string) string {
			switch node.Kind() {
			case "field_declaration":
				return "field"
			case "function_declaration":
				return "func"
			}
			return defaultNavigationDeclarationKind(node, container)
		},
		visibility: func(_ *syntaxNode, name, _ string) NavigationVisibility {
			return goNavigationVisibility(navigationTerminal(name))
		},
		sourceFacts: goNavigationSourceFacts, entrypoint: goNavigationEntrypoint,
		returnCallableName: func(node *syntaxNode, container, content string, _ *navigationAdapterConfig) string {
			if node.Kind() == "method_declaration" {
				return goNavigationMethodName(node, content, rules)
			}
			name := extractNodeName(node, content, rules)
			if container != "" && name != "" && !strings.Contains(name, ".") {
				return container + "." + name
			}
			return name
		},
		fieldNames: goNavigationFieldNames,
		callableReturnBinding: func(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "result")
		},
	}
	return adapter
}

func goNavigationEntrypoint(node *syntaxNode, name, container, packageName, _ string) string {
	if node.Kind() == "function_declaration" && name == "main" && container == "" && packageName == "main" {
		return "process"
	}
	return ""
}

func goNavigationFieldNames(node, typeNode *syntaxNode, content string) []navigationFieldName {
	names := defaultNavigationParameterNames(node, typeNode, content)
	result := make([]navigationFieldName, 0, max(1, len(names)))
	for _, name := range names {
		result = append(result, navigationFieldName{name: name})
	}
	if len(result) == 0 && node.Kind() == "field_declaration" {
		if name := navigationTypeName(typeNode.Text()); name != "" {
			result = append(result, navigationFieldName{name: name, embedded: true})
		}
	}
	return result
}

func ecmaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression", "new_expression"), extraContainerTypes: newStringSet("interface_declaration"),
		fieldContainerTypes: newStringSet("class_declaration", "interface_declaration"), selfBindingName: "this", selfBindingFromContainer: true,
		visibility: typeScriptAdapterVisibility, sourceFacts: typeScriptNavigationSourceFacts, exports: typeScriptNavigationExports,
		callableReturnBinding: func(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "return_type", "type")
		},
	}
}

func pythonNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call"), visibility: func(_ *syntaxNode, name, _ string) NavigationVisibility {
		return pythonNavigationVisibility(navigationTerminal(name))
	}}
}

func javaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("method_invocation", "object_creation_expression", "explicit_constructor_invocation"), visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return javaNavigationVisibility(node, navigationDeclarationHeader(node, content))
	}}
}

func kotlinNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return visibilityFromModifiers(navigationDeclarationHeader(node, content), true)
	}}
}

func cSharpNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("invocation_expression", "object_creation_expression"), visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return cSharpNavigationVisibility(navigationDeclarationHeader(node, content))
	}}
}

func cFamilyNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), declarationName: func(node *syntaxNode, content string, _ *navigationEnvelope) string {
		return cFamilyName(node, content, rules)
	}}
}

func rustNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), containerName: func(node *syntaxNode, content string, envelope *navigationEnvelope) string {
		if node.Kind() == "impl_item" {
			if target := node.ChildByFieldName("type"); target != nil {
				return target.Text()
			}
		}
		return defaultNavigationDeclarationName(node, content, rules, envelope)
	}, visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return visibilityFromRequiredModifier(navigationDeclarationHeader(node, content), "pub")
	}}
}

func shellNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("command")}
}

func defaultNavigationDeclarationName(node *syntaxNode, content string, rules *structureRules, envelope *navigationEnvelope) string {
	if envelope != nil && envelope.assignedName != "" {
		return envelope.assignedName
	}
	return extractNodeName(node, content, rules)
}

func defaultNavigationDeclarationKind(node *syntaxNode, container string) string {
	kind := node.Kind()
	if strings.Contains(kind, "constructor") {
		return "constructor"
	}
	if strings.Contains(kind, "method") || container != "" {
		return "method"
	}
	return "function"
}

func navigationTerminal(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func defaultNavigationTerminalName(node *syntaxNode) string {
	candidate := ""
	node.WalkNamed(func(current *syntaxNode) {
		switch current.Kind() {
		case "identifier", "field_identifier", "property_identifier", "type_identifier", "command_name", "word":
			candidate = current.Text()
		}
	})
	return candidate
}

var defaultNavigationWrapperTypes = newStringSet("export_statement", "decorated_definition", "lexical_declaration", "variable_declaration", "variable_declarator", "template_declaration")

var defaultNavigationNestedBindingScopes = newStringSet(
	"block", "statement_block", "if_statement", "for_statement", "for_in_statement", "while_statement", "do_statement",
	"switch_statement", "expression_switch_statement", "type_switch_statement", "select_statement", "try_statement", "catch_clause",
	"finally_clause", "expression_case", "type_case", "communication_case", "switch_case", "switch_default", "case_clause", "default_clause",
)

var defaultNavigationMemberTypes = newStringSet(
	"selector_expression", "member_expression", "attribute", "field_access", "member_access_expression", "field_expression", "navigation_expression",
)

var defaultNavigationFieldDeclarationTypes = newStringSet(
	"field_declaration", "public_field_definition", "field_definition", "property_signature", "abstract_property_signature",
)

var defaultNavigationParameterTypes = newStringSet(
	"parameter_declaration", "variadic_parameter_declaration", "required_parameter", "optional_parameter",
)

var defaultNavigationVariableBindingKinds = map[string]string{
	"variable_declarator":   "typed",
	"var_spec":              "typed",
	"short_var_declaration": "short",
}

func defaultNavigationParameterNames(node, typeNode *syntaxNode, _ string) []string {
	if name := node.ChildByFieldName("name"); name != nil {
		return []string{name.Text()}
	}
	if pattern := node.ChildByFieldName("pattern"); pattern != nil {
		return []string{pattern.Text()}
	}
	var names []string
	for _, child := range node.NamedChildren() {
		if child.ID() != typeNode.ID() && (child.Kind() == "identifier" || child.Kind() == "field_identifier") {
			names = append(names, child.Text())
		}
	}
	return names
}

func goNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	root.WalkNamed(func(node *syntaxNode) {
		switch node.Kind() {
		case "package_clause":
			packageName = navigationPackageName(node, content)
		case "import_spec":
			addGoNavigationImport(imports, node, content)
		}
	})
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, packageName, fields
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

func collectNavigationSourceFields(root *syntaxNode, content string, imports map[string]navigationImport, fields map[string]map[string]navigationBinding, adapter navigationAdapter) {
	root.WalkNamed(func(node *syntaxNode) {
		addNavigationContainerFields(fields, node, content, imports, adapter)
	})
}

func typeScriptAdapterVisibility(node *syntaxNode, name, content string) NavigationVisibility {
	return typeScriptNavigationVisibility(node, navigationTerminal(name), navigationDeclarationHeader(node, content))
}

func navigationReturnBindingFromFields(node *syntaxNode, _ string, imports map[string]navigationImport, fields ...string) navigationBinding {
	var result *syntaxNode
	for _, field := range fields {
		result = node.ChildByFieldName(field)
		if result != nil {
			break
		}
	}
	if result == nil {
		return navigationBinding{}
	}
	if result.Kind() == "parameter_list" || result.Kind() == "formal_parameters" {
		children := result.NamedChildren()
		if len(children) != 1 {
			return navigationBinding{}
		}
		result = children[0]
		if typed := result.ChildByFieldName("type"); typed != nil {
			result = typed
		}
	}
	return navigationBindingForType(result.Text(), imports)
}

func navigationPackageName(node *syntaxNode, _ string) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "package_identifier" || child.Kind() == "identifier" {
			return child.Text()
		}
	}
	return ""
}

func addGoNavigationImport(imports map[string]navigationImport, node *syntaxNode, _ string) {
	pathNode := node.ChildByFieldName("path")
	if pathNode == nil {
		return
	}
	importPath := unquoteNavigationPath(pathNode.Text())
	if importPath == "" {
		return
	}
	alias := path.Base(importPath)
	if name := node.ChildByFieldName("name"); name != nil {
		alias = name.Text()
	}
	if alias == "" {
		return
	}
	key := alias
	if alias == "_" || alias == "." {
		key += "\x00" + strconv.Itoa(node.StartLine())
	}
	imports[key] = navigationImport{alias: alias, path: importPath, imported: "*", line: node.StartLine()}
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

func navigationFirstIdentifier(node *syntaxNode, _ string) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "identifier" {
			return child.Text()
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
