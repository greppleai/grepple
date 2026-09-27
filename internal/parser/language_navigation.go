package parser

import (
	"strconv"
	"strings"
)

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

func addScopedNavigationImport(imports map[string]navigationImport, alias, importPath, imported string, line int) {
	if alias == "" || importPath == "" {
		return
	}
	item := navigationImport{alias: alias, path: importPath, imported: imported, line: line}
	if alias == "*" {
		imports[navigationImportUniqueKey(item)] = item
		return
	}
	if existing, ok := imports[alias]; ok {
		delete(imports, alias)
		imports[navigationImportUniqueKey(existing)] = existing
		imports[navigationImportUniqueKey(item)] = item
		return
	}
	for key := range imports {
		if strings.HasPrefix(key, alias+"\x00") {
			imports[navigationImportUniqueKey(item)] = item
			return
		}
	}
	imports[alias] = item
}

func addUnboundNavigationImport(imports map[string]navigationImport, alias, importPath, imported string, line int) {
	if alias == "" || importPath == "" {
		return
	}
	item := navigationImport{alias: alias, path: importPath, imported: imported, line: line}
	imports[navigationImportUniqueKey(item)] = item
}

func navigationImportUniqueKey(item navigationImport) string {
	return item.scope + "\x00" + item.alias + "\x00" + strconv.Itoa(item.line) + "\x00" + item.path + "\x00" + item.imported + "\x00" + item.kind
}

func navigationDirectiveValue(text, keyword string) string {
	value := strings.TrimSpace(text)
	if strings.HasPrefix(value, keyword) {
		value = strings.TrimSpace(strings.TrimPrefix(value, keyword))
	}
	return strings.TrimSpace(strings.TrimSuffix(value, ";"))
}

func compactNavigationQualifiedName(value string) string {
	return strings.Join(strings.Fields(value), "")
}

func collectNavigationSourceFields(root *syntaxNode, content string, imports map[string]navigationImport, fields map[string]map[string]navigationBinding, adapter navigationAdapter) {
	root.WalkNamed(func(node *syntaxNode) {
		addNavigationContainerFields(fields, node, content, imports, adapter)
	})
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
