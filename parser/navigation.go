package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// NavigationDeclaration describes a callable declaration found by a language adapter.
type NavigationDeclaration struct {
	ID        string
	Name      string
	Kind      string
	Language  string
	Path      string
	Container string
	Receiver  string
	Package   string
	PackageID string
	ModuleID  string
	Scope     string
	Start     int
	End       int
}

// NavigationCall describes a call and the callable declaration containing it.
type NavigationCall struct {
	ID             string
	CallerID       string
	TargetID       string
	Name           string
	Display        string
	Qualifier      string
	ImportPath     string
	ReceiverType   string
	ResolvedName   string
	Confidence     string
	Language       string
	Path           string
	Line           int
	EnclosingStart int
	EnclosingEnd   int
}

// NavigationGraph is the normalized, language-neutral callable and call model.
// Language-specific consumers may enrich its syntax facts with package, module,
// import, receiver, or type information.
type NavigationGraph struct {
	Declarations []NavigationDeclaration
	Calls        []NavigationCall
}

// Merge appends another source graph while preserving source and syntax order.
func (graph *NavigationGraph) Merge(other NavigationGraph) {
	graph.Declarations = append(graph.Declarations, other.Declarations...)
	graph.Calls = append(graph.Calls, other.Calls...)
}

func navigationStableID(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Navigation extracts callable declarations and calls for a structurally supported language.
// Resolution is intentionally syntax-based; callers decide whether a name is unique.
func Navigation(content, language string) ([]NavigationDeclaration, []NavigationCall) {
	graph := BuildNavigationGraph(content, language, "")
	return graph.Declarations, graph.Calls
}

// BuildNavigationGraph parses one source into the shared navigation graph.
func BuildNavigationGraph(content, language, path string) NavigationGraph {
	adapter := adapterForLanguage(language)
	if adapter == nil {
		return NavigationGraph{}
	}
	tree, err := parseTree(adapter, content)
	if err != nil {
		return NavigationGraph{}
	}
	defer tree.Close()
	return NavigationGraphFromTree(tree.RootNode(), content, language, path)
}

// NavigationGraphFromTree builds a graph from an already parsed compatible tree.
// It allows analyzers to share navigation extraction without parsing source twice.
func NavigationGraphFromTree(root *sitter.Node, content, language, path string) NavigationGraph {
	adapter := adapterForLanguage(language)
	if adapter == nil || root == nil {
		return NavigationGraph{}
	}
	imports, packageName := navigationSourceFacts(root, content, language)
	collector := navigationCollector{content: content, language: language, path: path, rules: adapter.Rules(), imports: imports, packageName: packageName}
	collector.walk(root, navigationWalkContext{})
	return NavigationGraph{Declarations: collector.declarations, Calls: collector.calls}
}

// DeclarationRangeAt returns the narrowest callable declaration containing line.
func DeclarationRangeAt(content, language string, line int) (int, int, bool) {
	declarations, _ := Navigation(content, language)
	var best NavigationDeclaration
	found := false
	for _, declaration := range declarations {
		if line < declaration.Start || line > declaration.End {
			continue
		}
		if !found || declaration.End-declaration.Start < best.End-best.Start {
			best = declaration
			found = true
		}
	}
	return best.Start, best.End, found
}

type navigationCollector struct {
	content      string
	language     string
	path         string
	rules        *structureRules
	imports      map[string]navigationImport
	packageName  string
	declarations []NavigationDeclaration
	calls        []NavigationCall
}

type navigationEnvelope struct {
	start        int
	end          int
	assignedName string
}

type navigationWalkContext struct {
	envelope  *navigationEnvelope
	container string
	bindings  map[string]navigationBinding
	callable  *NavigationDeclaration
}

func (c *navigationCollector) walk(node *sitter.Node, context navigationWalkContext) {
	current := c.enterNavigationNode(node, context)
	c.recordNavigationCall(node, current.callable, current.bindings)
	c.walkNavigationChildren(node, current)
}

func (c *navigationCollector) enterNavigationNode(node *sitter.Node, context navigationWalkContext) navigationWalkContext {
	current := context
	if c.isNavigationContainer(node.Kind()) {
		if name := c.containerName(node, context.envelope); name != "" {
			current.container = name
		}
	}
	if !c.isCallableNode(node) {
		return current
	}
	if c.isGoTypeMemberNode(node) && current.container == "" {
		return current
	}
	name := c.declarationName(node, context.envelope)
	if name == "" {
		return current
	}
	start, end := c.navigationDeclarationRange(node, context.envelope)
	if current.container != "" && !strings.Contains(name, ".") {
		name = current.container + "." + name
	}
	declaration := NavigationDeclaration{
		Name: name, Kind: c.navigationDeclarationKind(node, current.container), Language: c.language, Path: c.path, Container: current.container, Package: c.packageName, Start: start, End: end,
	}
	declaration.ID = navigationStableID("declaration", declaration.Language, declaration.Path, declaration.Name, declaration.Kind, strconv.Itoa(start), strconv.Itoa(end))
	c.declarations = append(c.declarations, declaration)
	current.callable = &c.declarations[len(c.declarations)-1]
	current.bindings = navigationCallableBindings(node, c.content, c.language, current.container, c.imports)
	return current
}

func (c *navigationCollector) navigationDeclarationRange(node *sitter.Node, envelope *navigationEnvelope) (int, int) {
	if envelope != nil {
		return envelope.start, envelope.end
	}
	return nodeStart(node), nodeEnd(node)
}

func (c *navigationCollector) recordNavigationCall(node *sitter.Node, callable *NavigationDeclaration, bindings map[string]navigationBinding) {
	if !c.isCall(node.Kind()) || callable == nil {
		return
	}
	name, display := c.callName(node)
	if name == "" {
		return
	}
	call := NavigationCall{
		Name: name, Display: display, Qualifier: navigationCallQualifier(display), Language: c.language, Path: c.path, Line: nodeStart(node),
		CallerID: callable.ID, EnclosingStart: callable.Start, EnclosingEnd: callable.End,
	}
	applyNavigationCallContext(&call, c.imports, bindings)
	call.ID = navigationStableID("call", call.CallerID, strconv.Itoa(call.Line), call.Display, strconv.Itoa(len(c.calls)))
	c.calls = append(c.calls, call)
}

func (c *navigationCollector) walkNavigationChildren(node *sitter.Node, context navigationWalkContext) {
	for _, child := range namedChildren(node) {
		childContext := context
		if navigationWrapperType(child.Kind()) {
			childContext.envelope = c.wrapperEnvelope(child, context.envelope)
		} else if !c.isCallableNode(child) {
			childContext.envelope = nil
		}
		if c.isCallableNode(child) && childContext.envelope == nil {
			childContext.envelope = &navigationEnvelope{start: leadingCommentStart(child), end: nodeEnd(child)}
		}
		c.walk(child, childContext)
	}
}
func (c *navigationCollector) wrapperEnvelope(node *sitter.Node, inherited *navigationEnvelope) *navigationEnvelope {
	assignedName := navigationAssignedName(node, c.content)
	if inherited == nil {
		return &navigationEnvelope{start: leadingCommentStart(node), end: nodeEnd(node), assignedName: assignedName}
	}
	wrapped := *inherited
	if assignedName != "" {
		wrapped.assignedName = assignedName
	}
	return &wrapped
}

func (c *navigationCollector) isCallableNode(node *sitter.Node) bool {
	if c.rules.functionLikeTypes.contains(node.Kind()) {
		return true
	}
	if c.language != "go" {
		return false
	}
	if node.Kind() == "method_elem" || node.Kind() == "method_spec" {
		return true
	}
	callableType := node.ChildByFieldName("type")
	return node.Kind() == "field_declaration" && callableType != nil && callableType.Kind() == "function_type"
}

func (c *navigationCollector) isGoTypeMemberNode(node *sitter.Node) bool {
	if c.language != "go" {
		return false
	}
	return node.Kind() == "method_elem" || node.Kind() == "method_spec" || node.Kind() == "field_declaration"
}
func (c *navigationCollector) isNavigationContainer(kind string) bool {
	if c.rules.classDeclarationTypes.contains(kind) {
		return true
	}
	if c.language == "go" && kind == "type_spec" {
		return true
	}
	return (c.language == "typescript" || c.language == "tsx") && kind == "interface_declaration"
}

func (c *navigationCollector) containerName(node *sitter.Node, envelope *navigationEnvelope) string {
	if c.language == "rust" && node.Kind() == "impl_item" {
		if target := node.ChildByFieldName("type"); target != nil {
			return nodeText(target, c.content)
		}
	}
	return c.declarationName(node, envelope)
}

func (c *navigationCollector) declarationName(node *sitter.Node, envelope *navigationEnvelope) string {
	if envelope != nil && envelope.assignedName != "" {
		return envelope.assignedName
	}
	if c.language == "c" || c.language == "cpp" {
		return cFamilyName(node, c.content, c.rules)
	}
	if c.language == "go" && node.Kind() == "method_declaration" {
		return goNavigationMethodName(node, c.content, c.rules)
	}
	return extractNodeName(node, c.content, c.rules)
}
func goNavigationMethodName(node *sitter.Node, content string, rules *structureRules) string {
	name := extractNodeName(node, content, rules)
	receiver := strings.TrimSuffix(strings.TrimPrefix(goReceiver(node, content), "("), ").")
	receiver = strings.TrimPrefix(receiver, "*")
	if receiver == "" {
		return name
	}
	return receiver + "." + name
}

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

func navigationWrapperType(kind string) bool {
	switch kind {
	case "export_statement", "decorated_definition", "lexical_declaration", "variable_declaration", "variable_declarator", "template_declaration":
		return true
	}
	return false
}

func (c *navigationCollector) navigationDeclarationKind(node *sitter.Node, container string) string {
	kind := node.Kind()
	if c.language == "go" {
		if kind == "field_declaration" {
			return "field"
		}
		if kind == "function_declaration" {
			return "func"
		}
	}
	if strings.Contains(kind, "constructor") {
		return "constructor"
	}
	if strings.Contains(kind, "method") || container != "" {
		return "method"
	}
	return "function"
}

func (c *navigationCollector) isCall(kind string) bool {
	switch c.language {
	case "go":
		return kind == "call_expression"
	case "javascript", "typescript", "tsx":
		return kind == "call_expression" || kind == "new_expression"
	case "python":
		return kind == "call"
	case "java":
		return kind == "method_invocation" || kind == "object_creation_expression" || kind == "explicit_constructor_invocation"
	case "kotlin":
		return kind == "call_expression"
	case "csharp":
		return kind == "invocation_expression" || kind == "object_creation_expression"
	case "c", "cpp", "rust":
		return kind == "call_expression"
	case "shell":
		return kind == "command"
	}
	return false
}

func (c *navigationCollector) callName(node *sitter.Node) (string, string) {
	var target *sitter.Node
	for _, field := range []string{"function", "name", "constructor", "type"} {
		if target = node.ChildByFieldName(field); target != nil {
			break
		}
	}
	if target == nil {
		children := namedChildren(node)
		if len(children) > 0 {
			target = children[0]
		}
	}
	if target == nil {
		return "", ""
	}
	name := navigationTerminalName(target, c.content)
	display := strings.Join(strings.Fields(nodeText(target, c.content)), " ")
	if len(display) > 80 {
		display = display[:77] + "..."
	}
	return name, display
}

func navigationCallQualifier(display string) string {
	display = strings.Join(strings.Fields(display), "")
	if index := strings.Index(display, "."); index > 0 {
		return strings.TrimSuffix(display[:index], "?")
	}
	return ""
}

func navigationTerminalName(node *sitter.Node, content string) string {
	candidate := ""
	walkNodes(node, func(current *sitter.Node) {
		switch current.Kind() {
		case "identifier", "field_identifier", "property_identifier", "type_identifier", "command_name", "word":
			candidate = nodeText(current, content)
		}
	})
	return candidate
}
