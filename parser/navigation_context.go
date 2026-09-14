package parser

import (
	"sort"
	"strings"
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
	role          string
	embedded      bool
	visibility    NavigationVisibility
	line          int
}

func emptyNavigationSourceFacts() (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	return make(map[string]navigationImport), "", make(map[string]map[string]navigationBinding)
}

func navigationReturnBindings(root *syntaxNode, content string, imports map[string]navigationImport, adapter navigationAdapter) map[string]navigationBinding {
	bindings := make(map[string]navigationBinding)
	ambiguous := make(map[string]bool)
	collectNavigationReturnBindings(root, "", content, imports, adapter, bindings, ambiguous)
	return bindings
}

func collectNavigationReturnBindings(node *syntaxNode, container, content string, imports map[string]navigationImport, adapter navigationAdapter, bindings map[string]navigationBinding, ambiguous map[string]bool) {
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
	for _, child := range node.NamedChildren() {
		collectNavigationReturnBindings(child, container, content, imports, adapter, bindings, ambiguous)
	}
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

func addNavigationContainerFields(fields map[string]map[string]navigationBinding, node *syntaxNode, content string, imports map[string]navigationImport, adapter navigationAdapter) {
	if !adapter.IsFieldContainer(node.Kind()) {
		return
	}
	name := navigationFieldText(node, "name", content)
	if name == "" {
		return
	}
	containerFields := make(map[string]navigationBinding)
	ownerVisibility := adapter.Visibility(node, name, "")
	node.WalkNamed(func(current *syntaxNode) {
		addNavigationFieldBinding(containerFields, current, name, ownerVisibility, content, imports, adapter)
	})
	if len(containerFields) > 0 {
		fields[name] = containerFields
	}
}

func addNavigationFieldBinding(fields map[string]navigationBinding, node *syntaxNode, owner string, ownerVisibility NavigationVisibility, content string, imports map[string]navigationImport, adapter navigationAdapter) {
	if !adapter.IsFieldDeclaration(node.Kind()) {
		return
	}
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	binding := navigationBindingForType(typeNode.Text(), imports)
	binding.line = node.StartLine()
	for _, field := range adapter.FieldNames(node, typeNode, content) {
		fieldBinding := binding
		fieldBinding.embedded = field.embedded
		fieldBinding.visibility = adapter.Visibility(node, field.name, owner)
		if ownerVisibility != NavigationVisibilityPublic {
			fieldBinding.visibility = NavigationVisibilityNonPublic
		}
		fields[field.name] = fieldBinding
	}
}

func navigationFieldFacts(fields map[string]map[string]navigationBinding, language, path, packageName string) []NavigationField {
	owners := make([]string, 0, len(fields))
	for owner := range fields {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	facts := []NavigationField{}
	for _, owner := range owners {
		names := make([]string, 0, len(fields[owner]))
		for name := range fields[owner] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			binding := fields[owner][name]
			facts = append(facts, NavigationField{OwnerType: owner, Name: name, Type: binding.typeName, ImportPath: binding.importPath, Language: language, Path: path, Package: packageName, Line: binding.line, Visibility: binding.visibility, Embedded: binding.embedded})
		}
	}
	return facts
}

func navigationFieldText(node *syntaxNode, field, _ string) string {
	child := node.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return child.Text()
}

func navigationCallableBindings(node *syntaxNode, content, container string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) map[string]navigationBinding {
	bindings := make(map[string]navigationBinding)
	if name, binding, ok := adapter.SelfBinding(container); ok {
		binding.role = "receiver"
		bindings[name] = binding
		bindings["this"] = navigationBinding{typeName: container, role: "receiver"}
	}
	for _, field := range []string{"receiver", "parameters"} {
		root := node.ChildByFieldName(field)
		if root == nil {
			continue
		}
		root.WalkNamed(func(current *syntaxNode) {
			addNavigationParameterBinding(bindings, current, content, imports, adapter)
		})
	}
	addNavigationLocalBindings(bindings, node.ChildByFieldName("body"), content, imports, returnBindings, adapter)
	return bindings
}

func addNavigationLocalBindings(bindings map[string]navigationBinding, root *syntaxNode, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) {
	if root == nil {
		return
	}
	for _, child := range root.NamedChildren() {
		if adapter.IsCallable(child) || adapter.IsNestedBindingScope(child.Kind()) {
			continue
		}
		addNavigationVariableBinding(bindings, child, content, imports, returnBindings, adapter)
		addNavigationLocalBindings(bindings, child, content, imports, returnBindings, adapter)
	}
}

func mergeNavigationBindingsAfterNode(bindings map[string]navigationBinding, node *syntaxNode, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) {
	if bindings == nil || adapter.IsCallable(node) || adapter.IsNestedBindingScope(node.Kind()) {
		return
	}
	discovered := make(map[string]navigationBinding)
	addNavigationVariableBinding(discovered, node, content, imports, returnBindings, adapter)
	addNavigationLocalBindings(discovered, node, content, imports, returnBindings, adapter)
	for name, binding := range discovered {
		bindings[name] = binding
	}
}

func addNavigationVariableBinding(bindings map[string]navigationBinding, node *syntaxNode, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) {
	switch adapter.VariableBindingKind(node.Kind()) {
	case "typed":
		name := node.ChildByFieldName("name")
		if name == nil {
			return
		}
		binding := navigationNodeBinding(node.ChildByFieldName("type"), node.ChildByFieldName("value"), content, imports, returnBindings, adapter)
		if navigationBindingKnown(binding) {
			addLocalNavigationBinding(bindings, name.Text(), binding, node.StartLine())
		}
	case "short":
		addNavigationShortVariableBindings(bindings, node, content, imports, returnBindings, adapter)
	}
}

func navigationBindingKnown(binding navigationBinding) bool {
	return binding.typeName != "" || binding.factoryName != ""
}

func addLocalNavigationBinding(bindings map[string]navigationBinding, name string, binding navigationBinding, line int) {
	if _, exists := bindings[name]; exists {
		return
	}
	binding.role = "local"
	binding.line = line
	bindings[name] = binding
}

func navigationNodeBinding(typeNode, valueNode *syntaxNode, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) navigationBinding {
	if typeNode != nil {
		return navigationBindingForType(typeNode.Text(), imports)
	}
	return navigationExpressionBinding(valueNode, content, imports, returnBindings, adapter)
}

func addNavigationShortVariableBindings(bindings map[string]navigationBinding, node *syntaxNode, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return
	}
	names := left.NamedChildren()
	values := right.NamedChildren()
	for index := 0; index < len(names) && index < len(values); index++ {
		binding := navigationExpressionBinding(values[index], content, imports, returnBindings, adapter)
		if navigationBindingKnown(binding) {
			addLocalNavigationBinding(bindings, names[index].Text(), binding, node.StartLine())
		}
	}
}

func navigationExpressionBinding(node *syntaxNode, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) navigationBinding {
	if node == nil {
		return navigationBinding{}
	}
	for _, field := range []string{"type", "constructor"} {
		if typeNode := node.ChildByFieldName(field); typeNode != nil {
			return navigationBindingForType(typeNode.Text(), imports)
		}
	}
	if binding, ok := navigationCallReturnBinding(node, content, imports, returnBindings, adapter); ok {
		return binding
	}
	children := node.NamedChildren()
	if len(children) == 1 {
		return navigationExpressionBinding(children[0], content, imports, returnBindings, adapter)
	}
	return navigationBinding{}
}

func navigationCallReturnBinding(node *syntaxNode, content string, imports map[string]navigationImport, returnBindings map[string]navigationBinding, adapter navigationAdapter) (navigationBinding, bool) {
	if !adapter.IsCall(node.Kind()) {
		return navigationBinding{}, false
	}
	target := node.ChildByFieldName("function")
	if target == nil {
		target = node.ChildByFieldName("name")
	}
	if target == nil {
		return navigationBinding{}, false
	}
	display := strings.Join(strings.Fields(target.Text()), "")
	if binding, ok := returnBindings[display]; ok {
		return binding, true
	}
	terminal := adapter.TerminalName(target, content)
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

func addNavigationParameterBinding(bindings map[string]navigationBinding, node *syntaxNode, content string, imports map[string]navigationImport, adapter navigationAdapter) {
	if !adapter.IsParameter(node.Kind()) {
		return
	}
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	binding := navigationBindingForType(typeNode.Text(), imports)
	if binding.typeName == "" {
		return
	}
	binding.role = "parameter"
	for _, name := range adapter.ParameterNames(node, typeNode, content) {
		binding.line = node.StartLine()
		bindings[name] = binding
	}
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
		if imported.imported != "" && imported.imported != "*" {
			binding.typeName = imported.imported
		}
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
	call.ReceiverRootType = binding.typeName
	call.ReceiverRootImport = binding.importPath
	call.ReceiverMembers = append([]string(nil), segments[1:len(segments)-1]...)
	for _, member := range call.ReceiverMembers {
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
