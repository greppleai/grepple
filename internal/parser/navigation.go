package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// NavigationDeclaration describes a callable declaration found by a language adapter.
type NavigationDeclaration struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Signature        string               `json:"signature,omitempty"`
	Kind             string               `json:"kind"`
	Language         string               `json:"language"`
	Path             string               `json:"path"`
	Container        string               `json:"container,omitempty"`
	Receiver         string               `json:"receiver,omitempty"`
	ResultType       string               `json:"resultType,omitempty"`
	ResultImportPath string               `json:"resultImportPath,omitempty"`
	Package          string               `json:"package,omitempty"`
	PackageID        string               `json:"packageId,omitempty"`
	ModuleID         string               `json:"moduleId,omitempty"`
	Scope            string               `json:"scope,omitempty"`
	Entrypoint       string               `json:"entrypoint,omitempty"`
	Visibility       NavigationVisibility `json:"visibility"`
	VisibilityDetail string               `json:"visibilityDetail,omitempty"`
	Start            int                  `json:"startLine"`
	End              int                  `json:"endLine"`
}

// NavigationCall describes a call and the callable declaration containing it.
type NavigationCall struct {
	ID                    string   `json:"id"`
	CallerID              string   `json:"callerId"`
	TargetID              string   `json:"targetId,omitempty"`
	CandidateTargetIDs    []string `json:"candidateTargetIds,omitempty"`
	Name                  string   `json:"name"`
	Display               string   `json:"display,omitempty"`
	Qualifier             string   `json:"qualifier,omitempty"`
	ImportPath            string   `json:"importPath,omitempty"`
	ReceiverType          string   `json:"receiverType,omitempty"`
	ReceiverRootType      string   `json:"receiverRootType,omitempty"`
	ReceiverRootImport    string   `json:"receiverRootImport,omitempty"`
	ReceiverMembers       []string `json:"receiverMembers,omitempty"`
	ReceiverFactory       string   `json:"receiverFactory,omitempty"`
	ReceiverFactoryImport string   `json:"receiverFactoryImport,omitempty"`
	ResolvedName          string   `json:"resolvedName,omitempty"`
	Confidence            string   `json:"confidence"`
	Language              string   `json:"language"`
	Path                  string   `json:"path"`
	Line                  int      `json:"line"`
	EnclosingStart        int      `json:"enclosingStartLine"`
	EnclosingEnd          int      `json:"enclosingEndLine"`
}

// NavigationImport records one source import recognized by a language adapter.
type NavigationImport struct {
	Alias            string   `json:"alias,omitempty"`
	ImportPath       string   `json:"importPath"`
	Imported         string   `json:"imported,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	Scope            string   `json:"scope,omitempty"`
	VisibilityDetail string   `json:"visibilityDetail,omitempty"`
	TargetPathHint   string   `json:"targetPathHint,omitempty"`
	Inline           bool     `json:"inline,omitempty"`
	Language         string   `json:"language"`
	Path             string   `json:"path"`
	Line             int      `json:"line"`
	TargetPaths      []string `json:"targetPaths,omitempty"`
}

// NavigationTypeUsage records a callable's explicit use of a normalized type.
type NavigationTypeUsage struct {
	CallerID   string `json:"callerId"`
	Type       string `json:"type"`
	ImportPath string `json:"importPath,omitempty"`
	Role       string `json:"role,omitempty"`
	Language   string `json:"language"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
}

// NavigationTypeDeclaration records a source-declared type and its complete syntax range.
type NavigationTypeDeclaration struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Language  string `json:"language"`
	Path      string `json:"path"`
	Container string `json:"container,omitempty"`
	Package   string `json:"package,omitempty"`
	PackageID string `json:"packageId,omitempty"`
	ModuleID  string `json:"moduleId,omitempty"`
	Start     int    `json:"startLine"`
	End       int    `json:"endLine"`
}

// NavigationExport describes a module export or re-export used for import resolution.
type NavigationExport struct {
	Name             string `json:"name"`
	LocalName        string `json:"localName,omitempty"`
	ImportPath       string `json:"importPath,omitempty"`
	ImportedName     string `json:"importedName,omitempty"`
	Scope            string `json:"scope,omitempty"`
	VisibilityDetail string `json:"visibilityDetail,omitempty"`
	Language         string `json:"language"`
	Path             string `json:"path"`
	Line             int    `json:"line"`
}

// NavigationField describes one typed field or property owned by a declared type.
type NavigationField struct {
	OwnerType  string               `json:"ownerType"`
	Name       string               `json:"name"`
	Type       string               `json:"type"`
	ImportPath string               `json:"importPath,omitempty"`
	Language   string               `json:"language"`
	Path       string               `json:"path"`
	Package    string               `json:"package,omitempty"`
	Line       int                  `json:"line"`
	Visibility NavigationVisibility `json:"visibility"`
	Embedded   bool                 `json:"embedded,omitempty"`
}

// NavigationMemberAccess describes a receiver-qualified field or property read or write.
type NavigationMemberAccess struct {
	ID           string `json:"id"`
	CallerID     string `json:"callerId"`
	ReceiverType string `json:"receiverType,omitempty"`
	Receiver     string `json:"receiver,omitempty"`
	Member       string `json:"member"`
	Operation    string `json:"operation"`
	Language     string `json:"language"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	StartByte    int    `json:"startByte,omitempty"`
}

// NavigationGraph is the normalized, language-neutral declaration, call, import,
// type, field, export, and member-access model. Language-specific consumers may
// enrich its syntax facts with package, module, receiver, or repository context.
type NavigationGraph struct {
	Declarations     []NavigationDeclaration     `json:"declarations"`
	TypeDeclarations []NavigationTypeDeclaration `json:"typeDeclarations,omitempty"`
	Imports          []NavigationImport          `json:"imports,omitempty"`
	Calls            []NavigationCall            `json:"calls"`
	Exports          []NavigationExport          `json:"exports,omitempty"`
	Fields           []NavigationField           `json:"fields,omitempty"`
	TypeUsages       []NavigationTypeUsage       `json:"typeUsages,omitempty"`
	MemberAccesses   []NavigationMemberAccess    `json:"memberAccesses,omitempty"`
	RepositoryRoots  []string                    `json:"repositoryRoots,omitempty"`
}

// Merge appends another source graph while preserving source and syntax order.
func (graph *NavigationGraph) Merge(other NavigationGraph) {
	graph.Declarations = append(graph.Declarations, other.Declarations...)
	graph.TypeDeclarations = append(graph.TypeDeclarations, other.TypeDeclarations...)
	graph.Imports = append(graph.Imports, other.Imports...)
	graph.Calls = append(graph.Calls, other.Calls...)
	graph.Exports = append(graph.Exports, other.Exports...)
	graph.Fields = append(graph.Fields, other.Fields...)
	graph.TypeUsages = append(graph.TypeUsages, other.TypeUsages...)
	graph.MemberAccesses = append(graph.MemberAccesses, other.MemberAccesses...)
	graph.RepositoryRoots = append(graph.RepositoryRoots, other.RepositoryRoots...)
}

func navigationStableID(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func navigationDeclarationStableID(declaration NavigationDeclaration) string {
	parts := []string{"declaration", declaration.Language, declaration.Path}
	if declaration.Scope != "" {
		parts = append(parts, declaration.Scope)
	}
	parts = append(parts, declaration.Name, declaration.Kind, strconv.Itoa(declaration.Start), strconv.Itoa(declaration.End))
	return navigationStableID(parts...)
}

// BuildNavigationGraph parses one source into the shared navigation graph.
func BuildNavigationGraph(content, language, path string) NavigationGraph {
	adapter := adapterForLanguage(language)
	if adapter == nil {
		return NavigationGraph{}
	}
	tree, err := adapter.Parse(content)
	if err != nil {
		return NavigationGraph{}
	}
	defer tree.Close()
	return navigationGraphFromTree(tree.RootNode(), content, language, path)
}

// navigationGraphFromDocument builds a graph from an already parsed document.
// The document remains owned by the caller and is not reparsed.
func navigationGraphFromDocument(document *Document, path string) NavigationGraph {
	if document == nil {
		return NavigationGraph{}
	}
	document.mu.RLock()
	defer document.mu.RUnlock()
	if document.tree == nil {
		return NavigationGraph{}
	}
	return navigationGraphFromTree(document.tree.RootNode(), document.source, document.language, path)
}

// navigationGraphFromTree builds a graph from an already parsed compatible tree.
// It allows analyzers to share navigation extraction without parsing source twice.
func navigationGraphFromTree(root *syntaxNode, content, language, path string) NavigationGraph {
	adapter := adapterForLanguage(language)
	navigation := navigationAdapterForLanguage(language)
	if adapter == nil || navigation == nil || root == nil {
		return NavigationGraph{}
	}
	imports, packageName, fields := navigation.SourceFacts(root, content)
	returnBindings := navigationReturnBindings(root, content, imports, navigation)
	collector := navigationCollector{content: content, adapter: adapter, navigation: navigation, path: path, imports: imports, fields: fields, returnBindings: returnBindings, packageName: packageName}
	collector.walk(root, navigationWalkContext{imports: navigationImportsAtScope(imports, "")})
	return NavigationGraph{Declarations: collector.declarations, TypeDeclarations: navigationTypeDeclarations(adapter.Outline(root, content), language, path, packageName), Calls: collector.calls, Imports: navigationImportFacts(imports, language, path), Exports: navigation.Exports(root, content, language, path), Fields: navigationFieldFacts(fields, language, path, packageName), TypeUsages: collector.typeUsages, MemberAccesses: collector.memberAccesses}
}

// DeclarationRangeAt returns the narrowest callable declaration containing line.
func DeclarationRangeAt(content, language string, line int) (int, int, bool) {
	graph := BuildNavigationGraph(content, language, "")
	return narrowestDeclarationRangeAt(graph.Declarations, line)
}

// DeclarationRangeAtFromDocument returns the narrowest callable declaration containing line
// without reparsing the caller-owned document.
func DeclarationRangeAtFromDocument(document *Document, path string, line int) (int, int, bool) {
	return narrowestDeclarationRangeAt(navigationGraphFromDocument(document, path).Declarations, line)
}

func narrowestDeclarationRangeAt(declarations []NavigationDeclaration, line int) (int, int, bool) {
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
	content        string
	adapter        languageAdapter
	navigation     navigationAdapter
	path           string
	imports        map[string]navigationImport
	fields         map[string]map[string]navigationBinding
	returnBindings map[string]navigationBinding
	packageName    string
	declarations   []NavigationDeclaration
	calls          []NavigationCall
	typeUsages     []NavigationTypeUsage
	memberAccesses []NavigationMemberAccess
}

type navigationEnvelope struct {
	start        int
	end          int
	assignedName string
}

type navigationWalkContext struct {
	envelope   *navigationEnvelope
	container  string
	modulePath string
	bindings   map[string]navigationBinding
	imports    map[string]navigationImport
	callable   *NavigationDeclaration
}

func (c *navigationCollector) walk(node *syntaxNode, context navigationWalkContext) {
	if c.navigation.IsNestedBindingScope(node.Kind()) && context.callable != nil {
		context.bindings = cloneNavigationBindings(context.bindings)
	}
	current := c.enterNavigationNode(node, context)
	c.recordNavigationMemberAccess(node, current)
	c.recordNavigationCall(node, current.callable, current.bindings, current.imports)
	c.walkNavigationChildren(node, current)
}

func cloneNavigationBindings(bindings map[string]navigationBinding) map[string]navigationBinding {
	cloned := make(map[string]navigationBinding, len(bindings))
	for name, binding := range bindings {
		cloned[name] = binding
	}
	return cloned
}

// navigationCallableSignature keeps the written declaration header, including
// parameters and return syntax, without confusing a body with a signature.
func navigationCallableSignature(node *syntaxNode) string {
	text := node.Text()
	if body := node.ChildByFieldName("body"); body != nil {
		end := int(body.StartByte()) - int(node.StartByte())
		if end <= 0 || end > len(text) {
			return ""
		}
		text = text[:end]
	}
	return strings.Join(strings.Fields(text), " ")
}

func (c *navigationCollector) enterNavigationNode(node *syntaxNode, context navigationWalkContext) navigationWalkContext {
	current := context
	if module := c.navigation.NestedModulePath(node, context.modulePath); module != "" {
		current.modulePath = module
		current.imports = navigationImportsAtScope(c.imports, current.modulePath)
	}
	if c.navigation.IsContainer(node.Kind()) {
		if name := c.navigation.ContainerName(node, c.content, context.envelope); name != "" {
			current.container = name
		}
	}
	if !c.navigation.IsCallable(node) {
		return current
	}
	if c.navigation.RequiresContainer(node) && current.container == "" {
		return current
	}
	name := c.navigation.DeclarationName(node, c.content, context.envelope)
	if name == "" {
		return current
	}
	start, end := c.navigationDeclarationRange(node, context.envelope)
	if current.container != "" && !strings.Contains(name, ".") {
		name = current.container + "." + name
	}
	result := c.navigation.CallableReturnBinding(node, c.content, current.imports)
	declaration := NavigationDeclaration{
		Name: name, Signature: navigationCallableSignature(node), Kind: c.navigation.DeclarationKind(node, current.container), Language: c.adapter.ID(), Path: c.path, Container: current.container, Package: c.packageName, Scope: current.modulePath,
		ResultType: result.typeName, ResultImportPath: result.importPath, Visibility: c.navigation.Visibility(node, name, c.content), VisibilityDetail: c.navigation.VisibilityDetail(node, name, c.content), Entrypoint: c.navigation.Entrypoint(navigationEntrypointContext{node: node, name: name, container: current.container, packageName: c.packageName, scope: current.modulePath, path: c.path, content: c.content}), Start: start, End: end,
	}
	declaration.ID = navigationDeclarationStableID(declaration)
	c.declarations = append(c.declarations, declaration)
	current.callable = &c.declarations[len(c.declarations)-1]
	current.bindings = navigationCallableBindings(node, c.content, current.container, current.imports, c.returnBindings, c.navigation)
	c.addNavigationSignatureBindings(node, current.bindings, current.imports)
	c.recordNavigationTypeUsages(current.callable, current.bindings)
	return current
}

func (c *navigationCollector) addNavigationSignatureBindings(node *syntaxNode, bindings map[string]navigationBinding, imports map[string]navigationImport) {
	body := node.ChildByFieldName("body")
	receiver := node.ChildByFieldName("receiver")
	node.WalkNamed(func(current *syntaxNode) {
		insideReceiver := receiver != nil && current.StartByte() >= receiver.StartByte() && current.EndByte() <= receiver.EndByte()
		if (body == nil || current.EndByte() <= body.StartByte()) && !insideReceiver {
			addNavigationParameterBinding(bindings, current, c.content, imports, c.navigation, "parameter")
		}
	})
}

func (c *navigationCollector) recordNavigationTypeUsages(callable *NavigationDeclaration, bindings map[string]navigationBinding) {
	type usageKey struct {
		typeName   string
		importPath string
		role       string
		line       int
	}
	usages := make(map[usageKey]bool)
	for _, binding := range bindings {
		typeName := strings.TrimSpace(binding.typeName)
		if typeName == "" {
			continue
		}
		line := binding.line
		if line == 0 {
			line = callable.Start
		}
		usages[usageKey{typeName: typeName, importPath: binding.importPath, role: binding.role, line: line}] = true
	}
	if callable.ResultType != "" {
		usages[usageKey{typeName: callable.ResultType, importPath: callable.ResultImportPath, role: "result", line: callable.Start}] = true
	}
	ordered := make([]usageKey, 0, len(usages))
	for usage := range usages {
		ordered = append(ordered, usage)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].typeName != ordered[j].typeName {
			return ordered[i].typeName < ordered[j].typeName
		}
		if ordered[i].importPath != ordered[j].importPath {
			return ordered[i].importPath < ordered[j].importPath
		}
		if ordered[i].role != ordered[j].role {
			return ordered[i].role < ordered[j].role
		}
		return ordered[i].line < ordered[j].line
	})
	for _, usage := range ordered {
		c.typeUsages = append(c.typeUsages, NavigationTypeUsage{CallerID: callable.ID, Type: usage.typeName, ImportPath: usage.importPath, Role: usage.role, Language: c.adapter.ID(), Path: c.path, Line: usage.line})
	}
}

func (c *navigationCollector) navigationDeclarationRange(node *syntaxNode, envelope *navigationEnvelope) (int, int) {
	if envelope != nil {
		return envelope.start, envelope.end
	}
	return node.StartLine(), node.EndLine()
}

func (c *navigationCollector) recordNavigationCall(node *syntaxNode, callable *NavigationDeclaration, bindings map[string]navigationBinding, imports map[string]navigationImport) {
	if !c.navigation.IsCall(node.Kind()) || callable == nil {
		return
	}
	name, display := c.callName(node)
	if name == "" {
		return
	}
	call := NavigationCall{
		Name: name, Display: display, Qualifier: navigationCallQualifier(display), Language: c.adapter.ID(), Path: c.path, Line: node.StartLine(),
		CallerID: callable.ID, EnclosingStart: callable.Start, EnclosingEnd: callable.End,
	}
	applyNavigationCallContext(&call, imports, bindings, c.fields)
	call.ID = navigationStableID("call", call.CallerID, strconv.Itoa(call.Line), call.Display, strconv.Itoa(len(c.calls)))
	c.calls = append(c.calls, call)
}

func (c *navigationCollector) recordNavigationMemberAccess(node *syntaxNode, context navigationWalkContext) {
	if context.callable == nil {
		return
	}
	syntax, ok := c.navigation.MemberAccess(node, c.content)
	if !ok {
		return
	}
	receiverType := ""
	if binding, exists := context.bindings[syntax.receiver]; exists {
		receiverType = binding.typeName
	} else if name, binding, exists := c.navigation.SelfBinding(context.container); exists && syntax.receiver == name {
		receiverType = binding.typeName
	} else if terminal := navigationTerminal(syntax.receiver); terminal != "" && strings.ToUpper(terminal[:1]) == terminal[:1] {
		receiverType = terminal
	}
	access := NavigationMemberAccess{
		CallerID: context.callable.ID, ReceiverType: receiverType, Receiver: syntax.receiver, Member: syntax.member, Operation: syntax.operation,
		Language: c.adapter.ID(), Path: c.path, Line: node.StartLine(), StartByte: int(node.StartByte()),
	}
	access.ID = navigationStableID("member-access", access.CallerID, strconv.Itoa(access.StartByte), access.Receiver, access.Member, access.Operation)
	c.memberAccesses = append(c.memberAccesses, access)
}

func (c *navigationCollector) walkNavigationChildren(node *syntaxNode, context navigationWalkContext) {
	for _, child := range node.NamedChildren() {
		childContext := context
		if c.navigation.IsWrapper(child.Kind()) {
			childContext.envelope = c.wrapperEnvelope(child, context.envelope)
		} else if !c.navigation.IsCallable(child) {
			childContext.envelope = nil
		}
		if c.navigation.IsCallable(child) && childContext.envelope == nil {
			childContext.envelope = &navigationEnvelope{start: leadingCommentStart(child), end: child.EndLine()}
		}
		c.walk(child, childContext)
		mergeNavigationBindingsAfterNode(context.bindings, child, c.content, context.imports, c.returnBindings, c.navigation)
	}
}
func (c *navigationCollector) wrapperEnvelope(node *syntaxNode, inherited *navigationEnvelope) *navigationEnvelope {
	assignedName := navigationAssignedName(node, c.content)
	if inherited == nil {
		return &navigationEnvelope{start: leadingCommentStart(node), end: node.EndLine(), assignedName: assignedName}
	}
	wrapped := *inherited
	if assignedName != "" {
		wrapped.assignedName = assignedName
	}
	return &wrapped
}

func (c *navigationCollector) callName(node *syntaxNode) (string, string) {
	var target *syntaxNode
	for _, field := range []string{"function", "name", "constructor", "type"} {
		if target = node.ChildByFieldName(field); target != nil {
			break
		}
	}
	if target == nil {
		children := node.NamedChildren()
		if len(children) > 0 {
			target = children[0]
		}
	}
	if target == nil {
		return "", ""
	}
	name := c.navigation.TerminalName(target, c.content)
	display := strings.Join(strings.Fields(c.navigation.CallDisplay(node, target, c.content)), " ")
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
	if index := strings.Index(display, "::"); index > 0 {
		return display[:index]
	}
	return ""
}
