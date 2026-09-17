package parser

import (
	"strings"
)

func navigationAssignedName(node *syntaxNode, _ string) string {
	if named := node.ChildByFieldName("name"); named != nil {
		return named.Text()
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() == "variable_declarator" {
			if named := child.ChildByFieldName("name"); named != nil {
				return named.Text()
			}
		}
	}
	return ""
}

type navigationFieldName struct {
	name     string
	embedded bool
}

type navigationMemberSyntax struct {
	receiver  string
	member    string
	operation string
}

type navigationEntrypointContext struct {
	node                         *syntaxNode
	name, container, packageName string
	scope, path, content         string
}

type navigationAdapter interface {
	Rules() *structureRules
	IsCallable(*syntaxNode) bool
	RequiresContainer(*syntaxNode) bool
	IsContainer(string) bool
	ContainerName(*syntaxNode, string, *navigationEnvelope) string
	DeclarationName(*syntaxNode, string, *navigationEnvelope) string
	DeclarationKind(*syntaxNode, string) string
	IsCall(string) bool
	IsWrapper(string) bool
	IsNestedBindingScope(string) bool
	NestedModulePath(*syntaxNode, string) string
	MemberAccess(*syntaxNode, string) (navigationMemberSyntax, bool)
	Visibility(*syntaxNode, string, string) NavigationVisibility
	VisibilityDetail(*syntaxNode, string, string) string
	SourceFacts(*syntaxNode, string) (map[string]navigationImport, string, map[string]map[string]navigationBinding)
	ImportFactsSupported() bool
	FactCapabilities() NavigationFactCapabilities
	ReturnCallableName(*syntaxNode, string, string) string
	CallableReturnBinding(*syntaxNode, string, map[string]navigationImport) navigationBinding
	IsFieldContainer(string) bool
	SelfBinding(string) (string, navigationBinding, bool)
	TerminalName(*syntaxNode, string) string
	CallDisplay(*syntaxNode, *syntaxNode, string) string
	IsFieldDeclaration(string) bool
	IsParameter(string) bool
	ParameterType(*syntaxNode) *syntaxNode
	VariableBindingKind(string) string
	ParameterNames(*syntaxNode, *syntaxNode, string) []string
	FieldNames(*syntaxNode, *syntaxNode, string) []navigationFieldName
	Exports(*syntaxNode, string, string, string) []NavigationExport
	Entrypoint(navigationEntrypointContext) string
	EntrypointFactsSupported() bool
}

type navigationAdapterConfig struct {
	rules                    *structureRules
	callTypes                stringSet
	parameterTypes           stringSet
	extraContainerTypes      stringSet
	fieldContainerTypes      stringSet
	wrapperTypes             stringSet
	nestedBindingScopeTypes  stringSet
	nestedModulePath         func(*syntaxNode, string) string
	isCallable               func(*syntaxNode) bool
	requiresContainer        func(*syntaxNode) bool
	containerName            func(*syntaxNode, string, *navigationEnvelope) string
	declarationName          func(*syntaxNode, string, *navigationEnvelope) string
	declarationKind          func(*syntaxNode, string) string
	visibility               func(*syntaxNode, string, string) NavigationVisibility
	visibilityDetail         func(*syntaxNode, string, string) string
	sourceFacts              func(*syntaxNode, string, *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding)
	returnCallableName       func(*syntaxNode, string, string, *navigationAdapterConfig) string
	callableReturnBinding    func(*syntaxNode, string, map[string]navigationImport) navigationBinding
	terminalName             func(*syntaxNode, string) string
	callDisplay              func(*syntaxNode, *syntaxNode, string) string
	selfBindingName          string
	selfBindingFromContainer bool
	parameterType            func(*syntaxNode) *syntaxNode
	fieldNames               func(*syntaxNode, *syntaxNode, string) []navigationFieldName
	exports                  func(*syntaxNode, string, string, string) []NavigationExport
	entrypoint               func(navigationEntrypointContext) string
	typeReferenceFacts       bool
	fieldFacts               bool
	memberAccessFacts        bool
}

func (adapter *navigationAdapterConfig) Entrypoint(context navigationEntrypointContext) string {
	if adapter.entrypoint == nil {
		return ""
	}
	return adapter.entrypoint(context)
}

func (adapter *navigationAdapterConfig) EntrypointFactsSupported() bool {
	return adapter.entrypoint != nil
}

func (adapter *navigationAdapterConfig) FactCapabilities() NavigationFactCapabilities {
	return NavigationFactCapabilities{
		Declarations:   adapter.rules != nil,
		Calls:          len(adapter.callTypes) > 0,
		Imports:        adapter.sourceFacts != nil,
		TypeReferences: adapter.typeReferenceFacts,
		Fields:         adapter.fieldFacts,
		MemberAccess:   adapter.memberAccessFacts,
		Entrypoints:    adapter.entrypoint != nil,
	}
}

func (adapter *navigationAdapterConfig) Rules() *structureRules { return adapter.rules }

func (adapter *navigationAdapterConfig) IsCallable(node *syntaxNode) bool {
	if adapter.isCallable != nil {
		return adapter.isCallable(node)
	}
	return adapter.rules.functionLikeTypes.contains(node.Kind())
}

func (adapter *navigationAdapterConfig) RequiresContainer(node *syntaxNode) bool {
	return adapter.requiresContainer != nil && adapter.requiresContainer(node)
}

func (adapter *navigationAdapterConfig) IsContainer(kind string) bool {
	return adapter.rules.classDeclarationTypes.contains(kind) || adapter.extraContainerTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) ContainerName(node *syntaxNode, content string, envelope *navigationEnvelope) string {
	if adapter.containerName != nil {
		return adapter.containerName(node, content, envelope)
	}
	return adapter.DeclarationName(node, content, envelope)
}

func (adapter *navigationAdapterConfig) DeclarationName(node *syntaxNode, content string, envelope *navigationEnvelope) string {
	if envelope != nil && envelope.assignedName != "" {
		return envelope.assignedName
	}
	if adapter.declarationName != nil {
		return adapter.declarationName(node, content, envelope)
	}
	return extractNodeName(node, content, adapter.rules)
}

func (adapter *navigationAdapterConfig) DeclarationKind(node *syntaxNode, container string) string {
	if adapter.declarationKind != nil {
		return adapter.declarationKind(node, container)
	}
	if strings.Contains(node.Kind(), "constructor") {
		return "constructor"
	}
	if strings.Contains(node.Kind(), "method") || container != "" {
		return "method"
	}
	return "function"
}

func (adapter *navigationAdapterConfig) IsCall(kind string) bool {
	return adapter.callTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) IsWrapper(kind string) bool {
	return defaultNavigationWrapperTypes.contains(kind) || adapter.wrapperTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) IsNestedBindingScope(kind string) bool {
	return defaultNavigationNestedBindingScopes.contains(kind) || adapter.nestedBindingScopeTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) NestedModulePath(node *syntaxNode, current string) string {
	if adapter.nestedModulePath == nil {
		return ""
	}
	return adapter.nestedModulePath(node, current)
}

func (adapter *navigationAdapterConfig) MemberAccess(node *syntaxNode, _ string) (navigationMemberSyntax, bool) {
	if node == nil || !defaultNavigationMemberTypes.contains(node.Kind()) {
		return navigationMemberSyntax{}, false
	}
	parent := node.Parent()
	if parent != nil && adapter.IsCall(parent.Kind()) {
		target := navigationFirstField(parent, "function", "name", "constructor", "type")
		if target != nil && target.StartByte() == node.StartByte() && target.EndByte() == node.EndByte() {
			return navigationMemberSyntax{}, false
		}
	}
	receiver := navigationFirstField(node, "object", "operand", "value", "expression", "primary", "receiver")
	member := navigationFirstField(node, "field", "property", "attribute", "name", "member")
	children := node.NamedChildren()
	if receiver == nil && len(children) > 0 {
		receiver = children[0]
	}
	if member == nil && len(children) > 1 {
		member = children[len(children)-1]
	}
	if receiver == nil || member == nil {
		return navigationMemberSyntax{}, false
	}
	receiverText := strings.TrimSpace(receiver.Text())
	memberText := strings.TrimSpace(member.Text())
	if receiverText == "" || memberText == "" || strings.ContainsAny(memberText, ".[]()") {
		return navigationMemberSyntax{}, false
	}
	return navigationMemberSyntax{receiver: receiverText, member: memberText, operation: navigationMemberOperation(node)}, true
}

func navigationMemberOperation(node *syntaxNode) string {
	for parent, depth := node.Parent(), 0; parent != nil && depth < 4; parent, depth = parent.Parent(), depth+1 {
		switch parent.Kind() {
		case "update_expression", "inc_statement", "dec_statement":
			return "write"
		case "assignment_expression", "assignment_statement", "assignment", "augmented_assignment", "short_var_declaration":
			left := navigationFirstField(parent, "left", "name")
			if left != nil && node.StartByte() >= left.StartByte() && node.EndByte() <= left.EndByte() {
				return "write"
			}
			return "read"
		}
	}
	return "read"
}

func navigationFirstField(node *syntaxNode, names ...string) *syntaxNode {
	for _, name := range names {
		if child := node.ChildByFieldName(name); child != nil {
			return child
		}
	}
	return nil
}

func (adapter *navigationAdapterConfig) Visibility(node *syntaxNode, name, content string) NavigationVisibility {
	if adapter.visibility == nil {
		return NavigationVisibilityUnknown
	}
	return adapter.visibility(node, name, content)
}

func (adapter *navigationAdapterConfig) VisibilityDetail(node *syntaxNode, name, content string) string {
	if adapter.visibilityDetail == nil {
		return ""
	}
	return adapter.visibilityDetail(node, name, content)
}

func (adapter *navigationAdapterConfig) ImportFactsSupported() bool {
	return adapter.sourceFacts != nil
}

func (adapter *navigationAdapterConfig) SourceFacts(root *syntaxNode, content string) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	if adapter.sourceFacts != nil {
		return adapter.sourceFacts(root, content, adapter)
	}
	return emptyNavigationSourceFacts()
}

func (adapter *navigationAdapterConfig) ReturnCallableName(node *syntaxNode, container, content string) string {
	if adapter.returnCallableName != nil {
		return adapter.returnCallableName(node, container, content, adapter)
	}
	name := extractNodeName(node, content, adapter.rules)
	if container != "" && name != "" && !strings.Contains(name, ".") {
		name = container + "." + name
	}
	return name
}

func (adapter *navigationAdapterConfig) CallableReturnBinding(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
	if adapter.callableReturnBinding == nil {
		return navigationBinding{}
	}
	return adapter.callableReturnBinding(node, content, imports)
}

func (adapter *navigationAdapterConfig) IsFieldContainer(kind string) bool {
	return adapter.fieldContainerTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) SelfBinding(container string) (string, navigationBinding, bool) {
	if !adapter.selfBindingFromContainer || container == "" {
		return "", navigationBinding{}, false
	}
	return adapter.selfBindingName, navigationBinding{typeName: container}, true
}

func (adapter *navigationAdapterConfig) TerminalName(node *syntaxNode, content string) string {
	if adapter.terminalName != nil {
		return adapter.terminalName(node, content)
	}
	return defaultNavigationTerminalName(node)
}

func (adapter *navigationAdapterConfig) CallDisplay(call, target *syntaxNode, content string) string {
	if adapter.callDisplay != nil {
		return adapter.callDisplay(call, target, content)
	}
	return target.Text()
}

func (*navigationAdapterConfig) IsFieldDeclaration(kind string) bool {
	return defaultNavigationFieldDeclarationTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) IsParameter(kind string) bool {
	return defaultNavigationParameterTypes.contains(kind) || adapter.parameterTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) ParameterType(node *syntaxNode) *syntaxNode {
	if adapter.parameterType != nil {
		return adapter.parameterType(node)
	}
	return node.ChildByFieldName("type")
}

func (*navigationAdapterConfig) VariableBindingKind(kind string) string {
	return defaultNavigationVariableBindingKinds[kind]
}

func (*navigationAdapterConfig) ParameterNames(node, typeNode *syntaxNode, content string) []string {
	return defaultNavigationParameterNames(node, typeNode, content)
}

func (adapter *navigationAdapterConfig) FieldNames(node, typeNode *syntaxNode, content string) []navigationFieldName {
	if adapter.fieldNames != nil {
		return adapter.fieldNames(node, typeNode, content)
	}
	names := adapter.ParameterNames(node, typeNode, content)
	result := make([]navigationFieldName, 0, len(names))
	for _, name := range names {
		result = append(result, navigationFieldName{name: name})
	}
	return result
}

func (adapter *navigationAdapterConfig) Exports(root *syntaxNode, content, language, path string) []NavigationExport {
	if adapter.exports == nil {
		return nil
	}
	return adapter.exports(root, content, language, path)
}
