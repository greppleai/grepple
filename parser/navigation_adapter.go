package parser

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func navigationAssignedName(node *sitter.Node, content string) string {
	if named := node.ChildByFieldName("name"); named != nil {
		return nodeText(named, content)
	}
	for _, child := range namedChildren(node) {
		if child.Kind() == "variable_declarator" {
			if named := child.ChildByFieldName("name"); named != nil {
				return nodeText(named, content)
			}
		}
	}
	return ""
}

type navigationMemberSyntax struct {
	receiver  string
	member    string
	operation string
}

type navigationAdapter interface {
	Rules() *structureRules
	IsCallable(*sitter.Node) bool
	RequiresContainer(*sitter.Node) bool
	IsContainer(string) bool
	ContainerName(*sitter.Node, string, *navigationEnvelope) string
	DeclarationName(*sitter.Node, string, *navigationEnvelope) string
	DeclarationKind(*sitter.Node, string) string
	IsCall(string) bool
	IsWrapper(string) bool
	IsNestedBindingScope(string) bool
	MemberAccess(*sitter.Node, string) (navigationMemberSyntax, bool)
	Visibility(*sitter.Node, string, string) NavigationVisibility
	SourceFacts(*sitter.Node, string) (map[string]navigationImport, string, map[string]map[string]navigationBinding)
	ReturnCallableName(*sitter.Node, string, string) string
	CallableReturnBinding(*sitter.Node, string, map[string]navigationImport) navigationBinding
	IsFieldContainer(string) bool
	SelfBinding(string) (string, navigationBinding, bool)
}

type navigationAdapterConfig struct {
	rules                    *structureRules
	callTypes                stringSet
	extraContainerTypes      stringSet
	fieldContainerTypes      stringSet
	wrapperTypes             stringSet
	nestedBindingScopeTypes  stringSet
	isCallable               func(*sitter.Node) bool
	requiresContainer        func(*sitter.Node) bool
	containerName            func(*sitter.Node, string, *navigationEnvelope) string
	declarationName          func(*sitter.Node, string, *navigationEnvelope) string
	declarationKind          func(*sitter.Node, string) string
	visibility               func(*sitter.Node, string, string) NavigationVisibility
	sourceFacts              func(*sitter.Node, string, *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding)
	returnCallableName       func(*sitter.Node, string, string, *navigationAdapterConfig) string
	callableReturnBinding    func(*sitter.Node, string, map[string]navigationImport) navigationBinding
	selfBindingName          string
	selfBindingFromContainer bool
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

func (adapter *navigationAdapterConfig) Rules() *structureRules { return adapter.rules }

func (adapter *navigationAdapterConfig) IsCallable(node *sitter.Node) bool {
	if adapter.isCallable != nil {
		return adapter.isCallable(node)
	}
	return adapter.rules.functionLikeTypes.contains(node.Kind())
}

func (adapter *navigationAdapterConfig) RequiresContainer(node *sitter.Node) bool {
	return adapter.requiresContainer != nil && adapter.requiresContainer(node)
}

func (adapter *navigationAdapterConfig) IsContainer(kind string) bool {
	return adapter.rules.classDeclarationTypes.contains(kind) || adapter.extraContainerTypes.contains(kind)
}

func (adapter *navigationAdapterConfig) ContainerName(node *sitter.Node, content string, envelope *navigationEnvelope) string {
	if adapter.containerName != nil {
		return adapter.containerName(node, content, envelope)
	}
	return adapter.DeclarationName(node, content, envelope)
}

func (adapter *navigationAdapterConfig) DeclarationName(node *sitter.Node, content string, envelope *navigationEnvelope) string {
	if envelope != nil && envelope.assignedName != "" {
		return envelope.assignedName
	}
	if adapter.declarationName != nil {
		return adapter.declarationName(node, content, envelope)
	}
	return extractNodeName(node, content, adapter.rules)
}

func (adapter *navigationAdapterConfig) DeclarationKind(node *sitter.Node, container string) string {
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

func (adapter *navigationAdapterConfig) MemberAccess(node *sitter.Node, content string) (navigationMemberSyntax, bool) {
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
	children := namedChildren(node)
	if receiver == nil && len(children) > 0 {
		receiver = children[0]
	}
	if member == nil && len(children) > 1 {
		member = children[len(children)-1]
	}
	if receiver == nil || member == nil {
		return navigationMemberSyntax{}, false
	}
	receiverText := strings.TrimSpace(nodeText(receiver, content))
	memberText := strings.TrimSpace(nodeText(member, content))
	if receiverText == "" || memberText == "" || strings.ContainsAny(memberText, ".[]()") {
		return navigationMemberSyntax{}, false
	}
	return navigationMemberSyntax{receiver: receiverText, member: memberText, operation: navigationMemberOperation(node)}, true
}

func navigationMemberOperation(node *sitter.Node) string {
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

func navigationFirstField(node *sitter.Node, names ...string) *sitter.Node {
	for _, name := range names {
		if child := node.ChildByFieldName(name); child != nil {
			return child
		}
	}
	return nil
}

func (adapter *navigationAdapterConfig) Visibility(node *sitter.Node, name, content string) NavigationVisibility {
	if adapter.visibility == nil {
		return NavigationVisibilityUnknown
	}
	return adapter.visibility(node, name, content)
}

func (adapter *navigationAdapterConfig) SourceFacts(root *sitter.Node, content string) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	if adapter.sourceFacts != nil {
		return adapter.sourceFacts(root, content, adapter)
	}
	return emptyNavigationSourceFacts()
}

func (adapter *navigationAdapterConfig) ReturnCallableName(node *sitter.Node, container, content string) string {
	if adapter.returnCallableName != nil {
		return adapter.returnCallableName(node, container, content, adapter)
	}
	name := extractNodeName(node, content, adapter.rules)
	if container != "" && name != "" && !strings.Contains(name, ".") {
		name = container + "." + name
	}
	return name
}

func (adapter *navigationAdapterConfig) CallableReturnBinding(node *sitter.Node, content string, imports map[string]navigationImport) navigationBinding {
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
