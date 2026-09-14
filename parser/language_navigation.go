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
		sourceFacts: goNavigationSourceFacts,
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
		callableReturnBinding: func(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "result")
		},
	}
	return adapter
}

func ecmaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression", "new_expression"), extraContainerTypes: newStringSet("interface_declaration"),
		fieldContainerTypes: newStringSet("class_declaration", "interface_declaration"), selfBindingName: "this", selfBindingFromContainer: true,
		visibility: typeScriptAdapterVisibility, sourceFacts: typeScriptNavigationSourceFacts,
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
	return imports, packageName, fields
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
	if alias == "" || alias == "_" || alias == "." {
		return
	}
	imports[alias] = navigationImport{path: importPath, imported: "*"}
}

func addTypeScriptNavigationImports(imports map[string]navigationImport, node *syntaxNode, content string) {
	source := node.ChildByFieldName("source")
	if source == nil {
		return
	}
	importPath := unquoteNavigationPath(source.Text())
	node.WalkNamed(func(current *syntaxNode) {
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
