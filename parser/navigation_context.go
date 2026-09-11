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
	typeName      string
	importPath    string
	factoryName   string
	factoryImport string
	line          int
}

func navigationSourceFacts(root *sitter.Node, content, language string) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
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
	fields := make(map[string]map[string]navigationBinding)
	walkNodes(root, func(node *sitter.Node) {
		addNavigationContainerFields(fields, node, content, language, imports)
	})
	return imports, packageName, fields
}

func navigationReturnBindings(root *sitter.Node, content, language string, imports map[string]navigationImport, rules *structureRules) map[string]navigationBinding {
	bindings := make(map[string]navigationBinding)
	ambiguous := make(map[string]bool)
	collectNavigationReturnBindings(root, "", content, language, imports, rules, bindings, ambiguous)
	return bindings
}

func collectNavigationReturnBindings(node *sitter.Node, container, content, language string, imports map[string]navigationImport, rules *structureRules, bindings map[string]navigationBinding, ambiguous map[string]bool) {
	if navigationFieldContainer(node.Kind(), language) {
		if name := extractNodeName(node, content, rules); name != "" {
			container = name
		}
	}
	if navigationBindingCallable(node.Kind(), language, rules) {
		name := navigationReturnCallableName(node, container, content, language, rules)
		binding := navigationCallableReturnBinding(node, content, language, imports)
		if name != "" && binding.typeName != "" {
			addNavigationReturnBinding(bindings, ambiguous, name, binding)
			if terminal := navigationReturnTerminal(name); terminal != name {
				addNavigationReturnBinding(bindings, ambiguous, terminal, binding)
			}
		}
		return
	}
	for _, child := range namedChildren(node) {
		collectNavigationReturnBindings(child, container, content, language, imports, rules, bindings, ambiguous)
	}
}

func navigationReturnCallableName(node *sitter.Node, container, content, language string, rules *structureRules) string {
	name := extractNodeName(node, content, rules)
	if language == "go" && node.Kind() == "method_declaration" {
		name = goNavigationMethodName(node, content, rules)
	} else if container != "" && name != "" && !strings.Contains(name, ".") {
		name = container + "." + name
	}
	return name
}

func navigationCallableReturnBinding(node *sitter.Node, content, language string, imports map[string]navigationImport) navigationBinding {
	var result *sitter.Node
	if language == "go" {
		result = node.ChildByFieldName("result")
	} else if language == "typescript" || language == "tsx" || language == "javascript" {
		result = node.ChildByFieldName("return_type")
		if result == nil {
			result = node.ChildByFieldName("type")
		}
	}
	if result == nil {
		return navigationBinding{}
	}
	if result.Kind() == "parameter_list" || result.Kind() == "formal_parameters" {
		children := namedChildren(result)
		if len(children) != 1 {
			return navigationBinding{}
		}
		result = children[0]
		if typed := result.ChildByFieldName("type"); typed != nil {
			result = typed
		}
	}
	return navigationBindingForType(nodeText(result, content), imports)
}

func addNavigationReturnBinding(bindings map[string]navigationBinding, ambiguous map[string]bool, name string, binding navigationBinding) {
	if ambiguous[name] {
		return
	}
	if _, exists := bindings[name]; exists {
		delete(bindings, name)
		ambiguous[name] = true
		return
	}
	bindings[name] = binding
}

func navigationReturnTerminal(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func addNavigationContainerFields(fields map[string]map[string]navigationBinding, node *sitter.Node, content, language string, imports map[string]navigationImport) {
	if !navigationFieldContainer(node.Kind(), language) {
		return
	}
	name := navigationFieldText(node, "name", content)
	if name == "" {
		return
	}
	containerFields := make(map[string]navigationBinding)
	walkNodes(node, func(current *sitter.Node) {
		addNavigationFieldBinding(containerFields, current, content, imports)
	})
	if len(containerFields) > 0 {
		fields[name] = containerFields
	}
}

func navigationFieldContainer(kind, language string) bool {
	if language == "go" {
		return kind == "type_spec"
	}
	if language == "typescript" || language == "tsx" || language == "javascript" {
		return kind == "class_declaration" || kind == "interface_declaration"
	}
	return false
}

func addNavigationFieldBinding(fields map[string]navigationBinding, node *sitter.Node, content string, imports map[string]navigationImport) {
	switch node.Kind() {
	case "field_declaration", "public_field_definition", "field_definition", "property_signature", "abstract_property_signature":
	default:
		return
	}
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	binding := navigationBindingForType(nodeText(typeNode, content), imports)
	for _, name := range navigationParameterNames(node, typeNode, content) {
		fields[name] = binding
	}
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

func navigationCallableBindings(node *sitter.Node, content, language, container string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, rules *structureRules) map[string]navigationBinding {
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
	addNavigationLocalBindings(bindings, node.ChildByFieldName("body"), content, language, imports, returnBindings, rules)
	return bindings
}

func addNavigationLocalBindings(bindings map[string]navigationBinding, root *sitter.Node, content, language string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, rules *structureRules) {
	if root == nil {
		return
	}
	for _, child := range namedChildren(root) {
		if navigationBindingCallable(child.Kind(), language, rules) || navigationNestedBindingScope(child.Kind()) {
			continue
		}
		addNavigationVariableBinding(bindings, child, content, imports, returnBindings)
		addNavigationLocalBindings(bindings, child, content, language, imports, returnBindings, rules)
	}
}

func mergeNavigationBindingsAfterNode(bindings map[string]navigationBinding, node *sitter.Node, content, language string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, rules *structureRules) {
	if bindings == nil || navigationBindingCallable(node.Kind(), language, rules) || navigationNestedBindingScope(node.Kind()) {
		return
	}
	discovered := make(map[string]navigationBinding)
	addNavigationVariableBinding(discovered, node, content, imports, returnBindings)
	addNavigationLocalBindings(discovered, node, content, language, imports, returnBindings, rules)
	for name, binding := range discovered {
		bindings[name] = binding
	}
}

func navigationBindingCallable(kind, language string, rules *structureRules) bool {
	if rules.functionLikeTypes.contains(kind) {
		return true
	}
	return language == "go" && (kind == "method_elem" || kind == "method_spec")
}

func navigationNestedBindingScope(kind string) bool {
	switch kind {
	case "block", "statement_block", "if_statement", "for_statement", "for_in_statement", "while_statement", "do_statement", "switch_statement", "expression_switch_statement", "type_switch_statement", "select_statement", "try_statement", "catch_clause", "finally_clause", "expression_case", "type_case", "communication_case", "switch_case", "switch_default", "case_clause", "default_clause":
		return true
	}
	return false
}

func addNavigationVariableBinding(bindings map[string]navigationBinding, node *sitter.Node, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding) {
	switch node.Kind() {
	case "variable_declarator", "var_spec":
		name := node.ChildByFieldName("name")
		if name == nil {
			return
		}
		binding := navigationNodeBinding(node.ChildByFieldName("type"), node.ChildByFieldName("value"), content, imports, returnBindings)
		if navigationBindingKnown(binding) {
			addLocalNavigationBinding(bindings, nodeText(name, content), binding, nodeStart(node))
		}
	case "short_var_declaration":
		addNavigationShortVariableBindings(bindings, node, content, imports, returnBindings)
	}
}

func navigationBindingKnown(binding navigationBinding) bool {
	return binding.typeName != "" || binding.factoryName != ""
}

func addLocalNavigationBinding(bindings map[string]navigationBinding, name string, binding navigationBinding, line int) {
	if _, exists := bindings[name]; exists {
		return
	}
	binding.line = line
	bindings[name] = binding
}

func navigationNodeBinding(typeNode, valueNode *sitter.Node, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding) navigationBinding {
	if typeNode != nil {
		return navigationBindingForType(nodeText(typeNode, content), imports)
	}
	return navigationExpressionBinding(valueNode, content, imports, returnBindings)
}

func addNavigationShortVariableBindings(bindings map[string]navigationBinding, node *sitter.Node, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return
	}
	names := namedChildren(left)
	values := namedChildren(right)
	for index := 0; index < len(names) && index < len(values); index++ {
		binding := navigationExpressionBinding(values[index], content, imports, returnBindings)
		if navigationBindingKnown(binding) {
			addLocalNavigationBinding(bindings, nodeText(names[index], content), binding, nodeStart(node))
		}
	}
}

func navigationExpressionBinding(node *sitter.Node, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding) navigationBinding {
	if node == nil {
		return navigationBinding{}
	}
	for _, field := range []string{"type", "constructor"} {
		if typeNode := node.ChildByFieldName(field); typeNode != nil {
			return navigationBindingForType(nodeText(typeNode, content), imports)
		}
	}
	if binding, ok := navigationCallReturnBinding(node, content, imports, returnBindings); ok {
		return binding
	}
	children := namedChildren(node)
	if len(children) == 1 {
		return navigationExpressionBinding(children[0], content, imports, returnBindings)
	}
	return navigationBinding{}
}

func navigationCallReturnBinding(node *sitter.Node, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding) (navigationBinding, bool) {
	if node.Kind() != "call_expression" {
		return navigationBinding{}, false
	}
	target := node.ChildByFieldName("function")
	if target == nil {
		target = node.ChildByFieldName("name")
	}
	if target == nil {
		return navigationBinding{}, false
	}
	display := strings.Join(strings.Fields(nodeText(target, content)), "")
	if binding, ok := returnBindings[display]; ok {
		return binding, true
	}
	terminal := navigationTerminalName(target, content)
	if binding, ok := returnBindings[terminal]; ok {
		return binding, true
	}
	binding := navigationBinding{factoryName: terminal}
	qualifier := navigationCallQualifier(display)
	if imported, ok := imports[display]; ok && imported.imported != "*" {
		binding.factoryName = imported.imported
		binding.factoryImport = imported.path
	} else if imported, ok := imports[qualifier]; ok {
		binding.factoryImport = imported.path
	}
	return binding, binding.factoryName != ""
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
		binding.line = nodeStart(node)
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

func applyNavigationCallContext(call *NavigationCall, imports map[string]navigationImport, bindings map[string]navigationBinding, fields map[string]map[string]navigationBinding) {
	if call.Qualifier != "" {
		applyQualifiedNavigationCallContext(call, imports, bindings, fields)
		return
	}
	if imported, ok := imports[call.Name]; ok && imported.imported != "*" {
		call.ImportPath = imported.path
		call.ResolvedName = imported.imported
	}
}

func applyQualifiedNavigationCallContext(call *NavigationCall, imports map[string]navigationImport, bindings map[string]navigationBinding, fields map[string]map[string]navigationBinding) {
	if imported, ok := imports[call.Qualifier]; ok {
		call.ImportPath = imported.path
		call.ResolvedName = call.Name
		return
	}
	segments := navigationMemberSegments(call.Display)
	if len(segments) < 2 {
		return
	}
	binding, ok := bindings[segments[0]]
	if !ok || binding.line > call.Line {
		return
	}
	for _, member := range segments[1 : len(segments)-1] {
		binding = fields[binding.typeName][member]
		if binding.typeName == "" {
			return
		}
	}
	if binding.typeName == "" {
		call.ReceiverFactory = binding.factoryName
		call.ReceiverFactoryImport = binding.factoryImport
		return
	}
	call.ReceiverType = binding.typeName
	call.ImportPath = binding.importPath
}

func navigationMemberSegments(display string) []string {
	compact := strings.ReplaceAll(strings.Join(strings.Fields(display), ""), "?", "")
	if strings.ContainsAny(compact, "()[]{}<>") {
		return nil
	}
	return strings.Split(compact, ".")
}
