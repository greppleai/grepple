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

func emptyNavigationSourceFacts() (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	return make(map[string]navigationImport), "", make(map[string]map[string]navigationBinding)
}

func navigationReturnBindings(root *sitter.Node, content string, imports map[string]navigationImport, adapter navigationAdapter) map[string]navigationBinding {
	bindings := make(map[string]navigationBinding)
	ambiguous := make(map[string]bool)
	collectNavigationReturnBindings(root, "", content, imports, adapter, bindings, ambiguous)
	return bindings
}

func collectNavigationReturnBindings(node *sitter.Node, container, content string, imports map[string]navigationImport, adapter navigationAdapter, bindings map[string]navigationBinding, ambiguous map[string]bool) {
	if adapter.IsFieldContainer(node.Kind()) {
		if name := extractNodeName(node, content, adapter.Rules()); name != "" {
			container = name
		}
	}
	if adapter.IsCallable(node) {
		name := adapter.ReturnCallableName(node, container, content)
		binding := adapter.CallableReturnBinding(node, content, imports)
		if name != "" && binding.typeName != "" {
			addNavigationReturnBinding(bindings, ambiguous, name, binding)
			if terminal := navigationReturnTerminal(name); terminal != name {
				addNavigationReturnBinding(bindings, ambiguous, terminal, binding)
			}
		}
		return
	}
	for _, child := range namedChildren(node) {
		collectNavigationReturnBindings(child, container, content, imports, adapter, bindings, ambiguous)
	}
}

func navigationReturnBindingFromFields(node *sitter.Node, content string, imports map[string]navigationImport, fields ...string) navigationBinding {
	var result *sitter.Node
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

func addNavigationContainerFields(fields map[string]map[string]navigationBinding, node *sitter.Node, content string, imports map[string]navigationImport, adapter navigationAdapter) {
	if !adapter.IsFieldContainer(node.Kind()) {
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

func navigationCallableBindings(node *sitter.Node, content, container string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) map[string]navigationBinding {
	bindings := make(map[string]navigationBinding)
	if name, binding, ok := adapter.SelfBinding(container); ok {
		bindings[name] = binding
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
	addNavigationLocalBindings(bindings, node.ChildByFieldName("body"), content, imports, returnBindings, adapter)
	return bindings
}

func addNavigationLocalBindings(bindings map[string]navigationBinding, root *sitter.Node, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) {
	if root == nil {
		return
	}
	for _, child := range namedChildren(root) {
		if adapter.IsCallable(child) || adapter.IsNestedBindingScope(child.Kind()) {
			continue
		}
		addNavigationVariableBinding(bindings, child, content, imports, returnBindings)
		addNavigationLocalBindings(bindings, child, content, imports, returnBindings, adapter)
	}
}

func mergeNavigationBindingsAfterNode(bindings map[string]navigationBinding, node *sitter.Node, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) {
	if bindings == nil || adapter.IsCallable(node) || adapter.IsNestedBindingScope(node.Kind()) {
		return
	}
	discovered := make(map[string]navigationBinding)
	addNavigationVariableBinding(discovered, node, content, imports, returnBindings)
	addNavigationLocalBindings(discovered, node, content, imports, returnBindings, adapter)
	for name, binding := range discovered {
		bindings[name] = binding
	}
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
