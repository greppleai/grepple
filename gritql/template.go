package gritql

import (
	"fmt"
	"github.com/greppleai/grepple/parser"
	"sort"
	"strconv"
	"strings"
)

// SnippetContext is the first Go grammar context in which a snippet parsed.
type SnippetContext uint8

// SnippetContextExpression and the following constants define Go snippet-context priority.
const (
	SnippetContextExpression SnippetContext = iota + 1
	SnippetContextType
	SnippetContextStatement
	SnippetContextStatementList
	SnippetContextDeclaration
	SnippetContextDeclarationList
	SnippetContextFile
)

func (c SnippetContext) String() string {
	names := [...]string{"", "expression", "type", "statement", "statement-list", "declaration", "declaration-list", "file"}
	if int(c) >= len(names) {
		return fmt.Sprintf("SnippetContext(%d)", c)
	}
	return names[c]
}

// SlotCardinality specifies whether a template slot binds one node or a node sequence.
type SlotCardinality uint8

// SlotOne and the following constants identify supported slot cardinalities.
const (
	SlotOne SlotCardinality = iota + 1
	SlotMany
)

type templateSlot struct {
	ref         VariableRef
	cardinality SlotCardinality
}
type templateChild struct {
	field string
	node  *templateNode
	slot  *templateSlot
}
type templateNode struct {
	kind     string
	named    bool
	token    string
	children []templateChild
}

// Template is an immutable structural pattern compiled in one Go snippet context.
type Template struct {
	context  SnippetContext
	root     *templateNode
	rootSlot *templateSlot
}

// Context returns the Go grammar context selected for the snippet.
func (t Template) Context() SnippetContext { return t.context }

// Root returns a read-only handle to the concrete template root, if present.
func (t Template) Root() TemplateNode { return TemplateNode{node: t.root} }

// RootSlot returns the root metavariable slot, if the template consists solely of one.
func (t Template) RootSlot() TemplateSlot { return TemplateSlot{slot: t.rootSlot} }

// Valid reports whether the template has a context and a concrete or slot root.
func (t Template) Valid() bool { return t.context != 0 && (t.root != nil || t.rootSlot != nil) }

// TemplateNode is a read-only handle to one concrete node in a structural template.
type TemplateNode struct{ node *templateNode }

// Valid reports whether the handle refers to a template node.
func (n TemplateNode) Valid() bool { return n.node != nil }

// Kind returns the pinned Go grammar node kind, or empty for an invalid handle.
func (n TemplateNode) Kind() string {
	if n.node == nil {
		return ""
	}
	return n.node.kind
}

// Named reports whether the grammar marks the node as named.
func (n TemplateNode) Named() bool { return n.node != nil && n.node.named }

// Token returns the exact significant token for a leaf node.
func (n TemplateNode) Token() string {
	if n.node == nil {
		return ""
	}
	return n.node.token
}

// Children returns read-only child handles in grammar order.
func (n TemplateNode) Children() []TemplateChild {
	if n.node == nil {
		return nil
	}
	out := make([]TemplateChild, len(n.node.children))
	for i := range n.node.children {
		out[i] = TemplateChild{child: &n.node.children[i]}
	}
	return out
}

// TemplateChild is a read-only handle to a field-associated node or slot.
type TemplateChild struct{ child *templateChild }

// Field returns the child's grammar field name, or empty when unfielded.
func (c TemplateChild) Field() string {
	if c.child == nil {
		return ""
	}
	return c.child.field
}

// IsSlot reports whether the child is a metavariable slot rather than a concrete node.
func (c TemplateChild) IsSlot() bool { return c.child != nil && c.child.slot != nil }

// Node returns the concrete node handle, or an invalid handle for a slot child.
func (c TemplateChild) Node() TemplateNode {
	if c.child == nil {
		return TemplateNode{}
	}
	return TemplateNode{node: c.child.node}
}

// Slot returns the metavariable slot handle, or an invalid handle for a concrete child.
func (c TemplateChild) Slot() TemplateSlot {
	if c.child == nil {
		return TemplateSlot{}
	}
	return TemplateSlot{slot: c.child.slot}
}

// TemplateSlot is a read-only handle to a metavariable position in a template.
type TemplateSlot struct{ slot *templateSlot }

// Valid reports whether the handle refers to a template slot.
func (s TemplateSlot) Valid() bool { return s.slot != nil }

// Variable returns the source metavariable reference represented by the slot.
func (s TemplateSlot) Variable() VariableRef {
	if s.slot == nil {
		return VariableRef{}
	}
	return s.slot.ref
}

// Cardinality returns whether the slot binds one node or a node sequence.
func (s TemplateSlot) Cardinality() SlotCardinality {
	if s.slot == nil {
		return 0
	}
	return s.slot.cardinality
}

type decodedSnippet struct {
	text         string
	placeholders []snippetPlaceholder
}
type snippetPlaceholder struct {
	start, end int
	ref        VariableRef
}

type placeholderRole uint8

const (
	roleNode placeholderRole = iota
	roleImportPath
	roleDeclarationSpec
	roleTypeSpec
	roleDeclaration
)

type generatedPlaceholder struct {
	start, end, index int
	ref               VariableRef
	role              placeholderRole
}
type spanKey struct{ start, end int }

type snippetAttempt struct {
	context        SnippetContext
	prefix, suffix string
	selectRoot     func(parser.Node, int, int) (selectedRoot, bool)
}
type selectedRoot struct {
	node         parser.Node
	sequenceKind string
}

var goSnippetAttempts = []snippetAttempt{
	{SnippetContextExpression, "package __grit_package\nvar __grit_value = ", "\n", selectExpression},
	{SnippetContextType, "package __grit_package\nvar __grit_value ", "\n", selectType},
	{SnippetContextStatement, "package __grit_package\nfunc __grit_func(){\n", "\n}\n", selectOneStatement},
	{SnippetContextStatementList, "package __grit_package\nfunc __grit_func(){\n", "\n}\n", selectStatementList},
	{SnippetContextDeclaration, "package __grit_package\n", "\n", selectOneDeclaration},
	{SnippetContextDeclarationList, "package __grit_package\n", "\n", selectDeclarationList},
	{SnippetContextFile, "", "", selectFile},
}

// Placeholder role inference has two phases per snippet context. First every
// occurrence is represented by a source-ordered unique ordinary identifier and
// one skeleton tree is inspected. Roles are then determined from actual Go
// ancestors and grammar-local import/spec components. Go's deterministic grammar
// currently yields one assignment; the production validation path accepts a list
// so future grammar-derived alternatives can be checked without changing it.

type placeholderInference struct {
	assignments [][]placeholderRole
	ok          bool
	tooDeep     bool
}

type structuralSeparators struct {
	left  []int // semicolon, newline, or opening parenthesis
	right []int // semicolon, newline, or closing parenthesis
}

// scanStructuralSeparators performs the one lexical pass used by all local
// component inference in a snippet context. Literal contents and same-line comment
// punctuation are deliberately absent. A newline within a block comment is present
// because Go treats it as a newline for semicolon insertion.
func scanStructuralSeparators(source string) structuralSeparators {
	s := separatorScanner{source: source, blockCommentStart: -1}
	s.scan()
	return s.result
}

type separatorLexState uint8

const (
	separatorLexNormal separatorLexState = iota
	separatorLexLineComment
	separatorLexBlockComment
	separatorLexInterpretedString
	separatorLexRawString
	separatorLexRune
)

type separatorScanner struct {
	source                 string
	result                 structuralSeparators
	state                  separatorLexState
	index                  int
	blockCommentStart      int
	blockCommentHasNewline bool
}

func (s *separatorScanner) scan() {
	for s.index = 0; s.index < len(s.source); s.index++ {
		s.scanCurrentByte()
	}
}

func (s *separatorScanner) scanCurrentByte() {
	switch s.state {
	case separatorLexLineComment:
		s.scanLineComment()
	case separatorLexBlockComment:
		s.scanBlockComment()
	case separatorLexRawString:
		s.scanRawString()
	case separatorLexInterpretedString, separatorLexRune:
		s.scanQuotedLiteral()
	default:
		s.scanNormalByte()
	}
}

func (s *separatorScanner) scanLineComment() {
	if s.source[s.index] != '\n' {
		return
	}
	s.addBothSeparator(s.index)
	s.state = separatorLexNormal
}

func (s *separatorScanner) scanBlockComment() {
	c := s.source[s.index]
	if c == '\n' {
		s.blockCommentHasNewline = true
	}
	if c != '*' || s.index+1 >= len(s.source) || s.source[s.index+1] != '/' {
		return
	}
	if s.blockCommentHasNewline {
		// A multiline block comment acts as the separator itself. Cutting at the
		// physical newline would leave an unmatched comment fragment.
		s.result.right = append(s.result.right, s.blockCommentStart)
		s.result.left = append(s.result.left, s.index+1)
	}
	s.index++
	s.state = separatorLexNormal
	s.blockCommentStart = -1
	s.blockCommentHasNewline = false
}

func (s *separatorScanner) scanRawString() {
	if s.source[s.index] == '`' {
		s.state = separatorLexNormal
	}
}

func (s *separatorScanner) scanQuotedLiteral() {
	c := s.source[s.index]
	if c == '\\' && s.index+1 < len(s.source) {
		s.index++
		return
	}
	if s.state == separatorLexInterpretedString && c == '"' || s.state == separatorLexRune && c == '\'' {
		s.state = separatorLexNormal
	}
}

func (s *separatorScanner) scanNormalByte() {
	c := s.source[s.index]
	if c == '/' && s.beginComment() {
		return
	}
	switch c {
	case '"':
		s.state = separatorLexInterpretedString
	case '\'':
		s.state = separatorLexRune
	case '`':
		s.state = separatorLexRawString
	case ';', '\n':
		s.addBothSeparator(s.index)
	case '(':
		s.result.left = append(s.result.left, s.index)
	case ')':
		s.result.right = append(s.result.right, s.index)
	}
}

func (s *separatorScanner) beginComment() bool {
	if s.index+1 >= len(s.source) {
		return false
	}
	switch s.source[s.index+1] {
	case '/':
		s.index++
		s.state = separatorLexLineComment
		return true
	case '*':
		s.blockCommentStart = s.index
		s.blockCommentHasNewline = false
		s.index++
		s.state = separatorLexBlockComment
		return true
	default:
		return false
	}
}

func (s *separatorScanner) addBothSeparator(at int) {
	s.result.left = append(s.result.left, at)
	s.result.right = append(s.result.right, at)
}

type roleAncestorSet struct {
	ranges [8]parser.Range
	set    [8]bool
}

var roleAncestorKinds = [...]string{"import_spec_list", "var_spec_list", "const_spec_list", "type_spec_list", "import_declaration", "var_declaration", "const_declaration", "type_declaration"}

func (a *roleAncestorSet) add(kind string, r parser.Range) {
	for i, candidate := range roleAncestorKinds {
		if kind == candidate {
			a.ranges[i], a.set[i] = r, true
			return
		}
	}
}

func (a roleAncestorSet) get(kind string) (parser.Range, bool) {
	for i, candidate := range roleAncestorKinds {
		if kind == candidate {
			return a.ranges[i], a.set[i]
		}
	}
	return parser.Range{}, false
}

type placeholderInfo struct {
	foundNode     bool
	topLevelValue bool
	ancestors     roleAncestorSet
	errorRange    parser.Range
}

func inferPlaceholderRoles(root parser.Node, ps []generatedPlaceholder, source string, maxDepth int) placeholderInference {
	infos, tooDeep := collectPlaceholderInfos(root, ps, maxDepth)
	if tooDeep {
		return placeholderInference{tooDeep: true}
	}
	state := placeholderRoleSolver{
		source:     source,
		ps:         ps,
		separators: scanStructuralSeparators(source),
		infos:      infos,
		roles:      make([]placeholderRole, len(ps)),
		resolved:   make([]bool, len(ps)),
	}
	state.resolveTopLevelValues()
	state.resolveImports()
	state.resolveWholeSpecs()
	state.resolveTypedNodes()
	state.resolveErrorRegions()
	if !state.allResolved() {
		return placeholderInference{}
	}
	return placeholderInference{assignments: completeInferenceAssignments(state.roles, nil), ok: true}
}

type placeholderWalkItem struct {
	n          parser.Node
	parent     parser.Node
	depth      int
	ancestors  roleAncestorSet
	inError    bool
	errorRange parser.Range
}

type placeholderInfoCollector struct {
	bySpan map[spanKey]int
	infos  []placeholderInfo
}

func collectPlaceholderInfos(root parser.Node, ps []generatedPlaceholder, maxDepth int) ([]placeholderInfo, bool) {
	c := placeholderInfoCollector{bySpan: make(map[spanKey]int, len(ps)), infos: make([]placeholderInfo, len(ps))}
	for _, p := range ps {
		c.bySpan[spanKey{p.start, p.end}] = p.index
	}
	stack := []placeholderWalkItem{{n: root, depth: 1}}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if x.depth > maxDepth {
			return nil, true
		}
		x = c.inspect(x)
		stack = appendPlaceholderChildren(stack, x)
	}
	return c.infos, false
}

func (c *placeholderInfoCollector) inspect(x placeholderWalkItem) placeholderWalkItem {
	r := x.n.Range()
	if x.n.IsError() || x.n.IsMissing() {
		x.inError, x.errorRange = true, r
	}
	if x.n.Valid() {
		x.ancestors.add(x.n.Kind(), r)
	}
	if i, ok := c.bySpan[spanKey{r.StartByte, r.EndByte}]; ok {
		c.record(i, x)
	}
	return x
}

func (c *placeholderInfoCollector) record(index int, x placeholderWalkItem) {
	info := &c.infos[index]
	info.ancestors = x.ancestors
	if x.inError {
		info.errorRange = x.errorRange
		return
	}
	if typedPlaceholderNode(x.n, x.parent, roleNode) && !info.foundNode {
		info.foundNode = true
		info.topLevelValue = x.n.Kind() == "expression_statement" && x.parent.Kind() == "source_file"
	}
}

func appendPlaceholderChildren(stack []placeholderWalkItem, x placeholderWalkItem) []placeholderWalkItem {
	children := x.n.Children()
	for i := len(children) - 1; i >= 0; i-- {
		stack = append(stack, placeholderWalkItem{
			n: children[i], parent: x.n, depth: x.depth + 1, ancestors: x.ancestors,
			inError: x.inError, errorRange: x.errorRange,
		})
	}
	return stack
}

type placeholderRoleSolver struct {
	source     string
	ps         []generatedPlaceholder
	separators structuralSeparators
	infos      []placeholderInfo
	roles      []placeholderRole
	resolved   []bool
}

func (s *placeholderRoleSolver) resolveTopLevelValues() {
	for i := range s.infos {
		if s.infos[i].topLevelValue {
			s.roles[i], s.resolved[i] = roleDeclaration, true
		}
	}
}

func (s *placeholderRoleSolver) resolveImports() {
	// An import spec has at most one alias and one path, so each component is
	// constant-sized and complete. Placeholder indexes remain source ordered.
	seen := make(map[spanKey]bool)
	for i := range s.infos {
		if s.resolved[i] {
			continue
		}
		r, ok := s.infos[i].ancestors.get("import_spec_list")
		if ok {
			s.resolveImport(i, r, seen)
		}
	}
}

func (s *placeholderRoleSolver) resolveImport(index int, enclosing parser.Range, seen map[spanKey]bool) {
	component := splitComponentRange(s.separators, enclosing, s.ps[index].start)
	key := spanKey{component.StartByte, component.EndByte}
	if seen[key] {
		return
	}
	seen[key] = true
	inferImportComponent(s.source, s.ps, component, index, s.roles, s.resolved)
}

func (s *placeholderRoleSolver) resolveWholeSpecs() {
	// Whole grouped specs take precedence over superficially valid recovery nodes.
	for i := range s.infos {
		if !s.resolved[i] {
			s.resolveWholeSpec(i)
		}
	}
}

func (s *placeholderRoleSolver) resolveWholeSpec(index int) {
	info := s.infos[index]
	if !placeholderIsWholeComponent(s.source, s.ps[index], nearestComponentRange(s.separators, info, s.ps[index])) {
		return
	}
	switch {
	case hasAncestor(info, "var_spec_list"), hasAncestor(info, "const_spec_list"):
		s.roles[index], s.resolved[index] = roleDeclarationSpec, true
	case hasAncestor(info, "type_spec_list"), hasAncestor(info, "type_declaration"):
		s.roles[index], s.resolved[index] = roleTypeSpec, true
	}
}

func (s *placeholderRoleSolver) resolveTypedNodes() {
	for i := range s.infos {
		if !s.resolved[i] && s.infos[i].foundNode {
			s.roles[i], s.resolved[i] = roleNode, true
		}
	}
}

func (s *placeholderRoleSolver) resolveErrorRegions() {
	for i := range s.infos {
		if !s.resolved[i] {
			s.resolveErrorRegion(i)
		}
	}
}

func (s *placeholderRoleSolver) resolveErrorRegion(index int) {
	keyword, region := errorDeclarationRegion(s.source, s.infos[index].errorRange, s.ps[index])
	if keyword == "import" {
		component := splitComponentRange(s.separators, region, s.ps[index].start)
		inferImportComponent(s.source, s.ps, component, index, s.roles, s.resolved)
		return
	}
	if !placeholderIsWholeComponent(s.source, s.ps[index], region) {
		return
	}
	switch keyword {
	case "var", "const":
		s.roles[index], s.resolved[index] = roleDeclarationSpec, true
	case "type":
		s.roles[index], s.resolved[index] = roleTypeSpec, true
	}
}

func (s *placeholderRoleSolver) allResolved() bool {
	for _, resolved := range s.resolved {
		if !resolved {
			return false
		}
	}
	return true
}

// roleAlternative is one correlated role change in a grammar-local component.
// All changes for the same component are emitted in one complete assignment: an
// import alias/path interpretation, for example, must change both occurrences.
type roleAlternative struct {
	component spanKey
	index     int
	role      placeholderRole
}

// completeInferenceAssignments emits the base and at most one alternative per
// grammar-local component. It never constructs a Cartesian product: given a valid
// base whole parse, disjoint Go grammar components cannot require simultaneous
// changes to expose a distinct interpretation.
func completeInferenceAssignments(base []placeholderRole, alternatives []roleAlternative) [][]placeholderRole {
	assignments := [][]placeholderRole{append([]placeholderRole(nil), base...)}
	componentIndexes := make(map[spanKey]int, len(alternatives))
	for _, change := range alternatives {
		if change.index < 0 || change.index >= len(base) {
			continue
		}
		assignmentIndex, ok := componentIndexes[change.component]
		if !ok {
			assignmentIndex = len(assignments)
			componentIndexes[change.component] = assignmentIndex
			assignments = append(assignments, append([]placeholderRole(nil), base...))
		}
		assignments[assignmentIndex][change.index] = change.role
	}
	return assignments
}

func hasAncestor(info placeholderInfo, kind string) bool {
	_, ok := info.ancestors.get(kind)
	return ok
}

func nearestComponentRange(separators structuralSeparators, info placeholderInfo, p generatedPlaceholder) parser.Range {
	for _, kind := range roleAncestorKinds {
		if r, ok := info.ancestors.get(kind); ok {
			return splitComponentRange(separators, r, p.start)
		}
	}
	return info.errorRange
}

func splitComponentRange(separators structuralSeparators, enclosing parser.Range, at int) parser.Range {
	start, end := enclosing.StartByte, enclosing.EndByte
	i := sort.SearchInts(separators.left, at) - 1
	if i >= 0 && separators.left[i] >= start {
		start = separators.left[i] + 1
	}
	i = sort.SearchInts(separators.right, at)
	if i < len(separators.right) && separators.right[i] < end {
		end = separators.right[i]
	}
	return parser.Range{StartByte: start, EndByte: end}
}

func inferImportComponent(source string, ps []generatedPlaceholder, r parser.Range, target int, roles []placeholderRole, resolved []bool) {
	first, last := target, target
	for first > 0 && ps[first-1].start >= r.StartByte && ps[first-1].end <= r.EndByte {
		first--
	}
	for last+1 < len(ps) && ps[last+1].start >= r.StartByte && ps[last+1].end <= r.EndByte {
		last++
	}
	if last-first+1 > 2 {
		return
	}
	after := stripGoTrivia(source[ps[last].end:r.EndByte])
	if strings.HasPrefix(after, "\"") || strings.HasPrefix(after, "`") {
		if first == last {
			roles[last], resolved[last] = roleNode, true
		}
		return
	}
	roles[last], resolved[last] = roleImportPath, true
	if first != last {
		roles[first], resolved[first] = roleNode, true
	}
}

func errorDeclarationRegion(source string, r parser.Range, p generatedPlaceholder) (string, parser.Range) {
	if r.EndByte <= r.StartByte || p.start < r.StartByte || p.end > r.EndByte {
		return "", parser.Range{}
	}
	s := stripGoTrivia(source[r.StartByte:r.EndByte])
	for _, keyword := range []string{"import", "var", "const", "type"} {
		if strings.HasPrefix(s, keyword) {
			return keyword, r
		}
	}
	return "", parser.Range{}
}

func placeholderIsWholeComponent(source string, p generatedPlaceholder, r parser.Range) bool {
	if r.EndByte <= r.StartByte || p.start < r.StartByte || p.end > r.EndByte {
		return false
	}
	rest := source[r.StartByte:p.start] + source[p.end:r.EndByte]
	rest = stripGoTrivia(rest)
	rest = strings.TrimSpace(rest)
	for _, keyword := range []string{"import", "var", "const", "type"} {
		rest = strings.TrimSpace(strings.TrimPrefix(rest, keyword))
	}
	rest = strings.Trim(rest, " \t\r\n();")
	return rest == ""
}

func stripGoTrivia(s string) string {
	for {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "//") {
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = s[i+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if i := strings.Index(s[2:], "*/"); i >= 0 {
				s = s[i+4:]
				continue
			}
			// A grammar component may end at a newline inside this comment.
			// Everything through that boundary is trivia even though the local
			// slice intentionally does not contain the closing delimiter.
			return ""
		}
		return s
	}
}

func compileGoTemplate(decoded decodedSnippet, maxDepth int) (Template, string, error) {
	for _, attempt := range goSnippetAttempts {
		baseline, generated := replaceSnippetPlaceholders(decoded, attempt.prefix, nil)
		source := attempt.prefix + baseline + attempt.suffix
		doc, err := parser.ParseDocument("go", source)
		if err != nil {
			continue
		}
		inference := inferPlaceholderRoles(doc.Root(), generated, source, maxDepth+16)
		doc.Close()
		if inference.tooDeep {
			return Template{}, "LIMIT_PARSE_DEPTH", fmt.Errorf("Go template exceeds effective depth limit")
		}
		if !inference.ok {
			continue
		}
		candidates, tooDeep := parseInferredTemplates(decoded, attempt, inference.assignments, maxDepth)
		if tooDeep {
			return Template{}, "LIMIT_PARSE_DEPTH", fmt.Errorf("Go template exceeds effective depth limit")
		}
		if len(candidates) > 1 {
			return Template{}, "PATTERN_AMBIGUOUS_SNIPPET", fmt.Errorf("snippet has multiple grammar-derived assignments in %s context", attempt.context)
		}
		if len(candidates) == 1 {
			return candidates[0], "", nil
		}
	}
	return Template{}, "PATTERN_INVALID_SNIPPET", fmt.Errorf("snippet is not valid Go in any supported context")
}

// collectInferredTemplates validates and canonicalizes the complete assignments
// returned by grammar inference. Assignment construction is kept in inference,
// rather than synthesized here, so this stage cannot miss a local alternative.
type inferredEvaluation struct {
	tmpl           Template
	valid, tooDeep bool
}

func collectInferredTemplates(occurrences int, assignments [][]placeholderRole, evaluate func([]placeholderRole) inferredEvaluation) ([]Template, bool) {
	var candidates []Template
	for _, roles := range assignments {
		if len(roles) != occurrences {
			continue
		}
		e := evaluate(roles)
		if e.tooDeep {
			return nil, true
		}
		if e.valid {
			candidates = append(candidates, e.tmpl)
		}
	}
	return dedupeTemplates(candidates), false
}

func parseInferredTemplates(decoded decodedSnippet, attempt snippetAttempt, assignments [][]placeholderRole, maxDepth int) ([]Template, bool) {
	evaluate := func(roles []placeholderRole) inferredEvaluation {
		replaced, generated := replaceSnippetPlaceholders(decoded, attempt.prefix, roles)
		source := attempt.prefix + replaced + attempt.suffix
		doc, err := parser.ParseDocument("go", source)
		if err != nil {
			return inferredEvaluation{}
		}
		defer doc.Close()
		root := doc.Root()
		valid, tooDeep := validTemplateParse(root, maxDepth+16)
		if tooDeep {
			return inferredEvaluation{tooDeep: true}
		}
		start, end := len(attempt.prefix), len(attempt.prefix)+len(replaced)
		sel, ok := attempt.selectRoot(root, start, end)
		if !ok || !valid || !consumesSnippet(sel.node, root, source, start, end) {
			return inferredEvaluation{}
		}
		frozen, buildErr := freezeTemplate(sel.node, generated, maxDepth, start, end, sel.sequenceKind)
		if buildErr != nil {
			return inferredEvaluation{tooDeep: strings.Contains(buildErr.Error(), "depth")}
		}
		t := Template{context: attempt.context, root: frozen.node, rootSlot: frozen.slot}
		return inferredEvaluation{tmpl: t, valid: restoredOccurrences(t, generated)}
	}
	return collectInferredTemplates(len(decoded.placeholders), assignments, evaluate)
}

func replaceSnippetPlaceholders(d decodedSnippet, prefix string, roles []placeholderRole) (string, []generatedPlaceholder) {
	if len(d.placeholders) == 0 {
		return d.text, nil
	}
	var b strings.Builder
	b.Grow(len(d.text) + len(d.placeholders)*24)
	out := make([]generatedPlaceholder, 0, len(d.placeholders))
	at := 0
	for i, p := range d.placeholders {
		b.WriteString(d.text[at:p.start])
		marker := "__grit_slot_" + strconv.Itoa(i) + "__"
		expansion := marker
		role := roleNode
		if i < len(roles) {
			role = roles[i]
		}
		switch role {
		case roleImportPath:
			expansion = "\"" + marker + "\""
		case roleDeclarationSpec:
			expansion = marker + " = 0"
		case roleTypeSpec:
			expansion = marker + " int"
		case roleDeclaration:
			expansion = "var " + marker + " int"
		}
		start := len(prefix) + b.Len()
		b.WriteString(expansion)
		end := len(prefix) + b.Len()
		out = append(out, generatedPlaceholder{start: start, end: end, index: i, ref: p.ref, role: role})
		at = p.end
	}
	b.WriteString(d.text[at:])
	return b.String(), out
}

func validTemplateParse(root parser.Node, maxDepth int) (bool, bool) {
	type item struct {
		n     parser.Node
		depth int
	}
	stack := []item{{root, 1}}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if x.depth > maxDepth {
			return false, true
		}
		if x.n.IsError() || x.n.IsMissing() {
			return false, false
		}
		for _, c := range x.n.Children() {
			stack = append(stack, item{c, x.depth + 1})
		}
	}
	return true, false
}
func directNamed(root parser.Node) []parser.Node {
	out := make([]parser.Node, 0, root.NamedChildCount())
	for _, n := range root.NamedChildren() {
		if !n.IsExtra() && n.Kind() != "comment" {
			out = append(out, n)
		}
	}
	return out
}
func childKind(n parser.Node, kind string) parser.Node {
	for _, c := range n.Children() {
		if c.Kind() == kind && !c.IsExtra() {
			return c
		}
	}
	return parser.Node{}
}
func selectExpression(root parser.Node, start, end int) (selectedRoot, bool) {
	n := directNamed(root)
	if len(n) != 2 || n[0].Kind() != "package_clause" || n[1].Kind() != "var_declaration" {
		return selectedRoot{}, false
	}
	spec := childKind(n[1], "var_spec")
	list := childKind(spec, "expression_list")
	kids := directNamed(list)
	if len(kids) != 1 || !rangeWithin(kids[0], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: kids[0]}, true
}
func selectType(root parser.Node, start, end int) (selectedRoot, bool) {
	n := directNamed(root)
	if len(n) != 2 || n[1].Kind() != "var_declaration" {
		return selectedRoot{}, false
	}
	typ := childKind(n[1], "var_spec").ChildByFieldName("type")
	return selectedRoot{node: typ}, rangeWithin(typ, start, end)
}
func statementList(root parser.Node) (parser.Node, bool) {
	n := directNamed(root)
	if len(n) != 2 || n[1].Kind() != "function_declaration" {
		return parser.Node{}, false
	}
	list := childKind(n[1].ChildByFieldName("body"), "statement_list")
	return list, list.Valid()
}
func selectOneStatement(root parser.Node, start, end int) (selectedRoot, bool) {
	list, ok := statementList(root)
	if !ok {
		return selectedRoot{}, false
	}
	kids := directNamed(list)
	if len(kids) != 1 || isGoDeclaration(kids[0].Kind()) || !rangeWithin(kids[0], start, end) {
		return selectedRoot{}, false
	}
	if hasExplicitSemicolon(list, start, end) {
		return selectedRoot{node: list, sequenceKind: "statement_sequence"}, true
	}
	return selectedRoot{node: kids[0]}, true
}
func selectStatementList(root parser.Node, start, end int) (selectedRoot, bool) {
	list, ok := statementList(root)
	kids := directNamed(list)
	if !ok || len(kids) < 2 || containsGoDeclaration(kids) || !rangeWithin(kids[0], start, end) || !rangeWithin(kids[len(kids)-1], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: list, sequenceKind: "statement_sequence"}, true
}
func rangeWithin(n parser.Node, start, end int) bool {
	r := n.Range()
	return n.Valid() && r.StartByte >= start && r.EndByte <= end && r.StartByte < r.EndByte
}
func hasExplicitSemicolon(n parser.Node, start, end int) bool {
	for _, c := range n.Children() {
		r := c.Range()
		if c.Kind() == ";" && r.StartByte >= start && r.EndByte <= end && r.EndByte > r.StartByte {
			return true
		}
	}
	return false
}
func isGoDeclaration(k string) bool {
	switch k {
	case "import_declaration", "const_declaration", "type_declaration", "var_declaration", "function_declaration", "method_declaration":
		return true
	}
	return false
}
func containsGoDeclaration(ns []parser.Node) bool {
	for _, n := range ns {
		if isGoDeclaration(n.Kind()) {
			return true
		}
	}
	return false
}
func declarations(root parser.Node) []parser.Node {
	n := directNamed(root)
	if len(n) == 0 || n[0].Kind() != "package_clause" {
		return nil
	}
	return n[1:]
}
func selectOneDeclaration(root parser.Node, start, end int) (selectedRoot, bool) {
	ds := declarations(root)
	if len(ds) != 1 || !isGoDeclaration(ds[0].Kind()) || !rangeWithin(ds[0], start, end) {
		return selectedRoot{}, false
	}
	if hasExplicitSemicolon(root, start, end) {
		return selectedRoot{node: root, sequenceKind: "declaration_sequence"}, true
	}
	return selectedRoot{node: ds[0]}, true
}
func selectDeclarationList(root parser.Node, start, end int) (selectedRoot, bool) {
	ds := declarations(root)
	if len(ds) < 2 || !allGoDeclarations(ds) || ds[0].Range().StartByte < start || ds[len(ds)-1].Range().EndByte > end {
		return selectedRoot{}, false
	}
	return selectedRoot{node: root, sequenceKind: "declaration_sequence"}, true
}
func selectFile(root parser.Node, start, end int) (selectedRoot, bool) {
	n := directNamed(root)
	ok := start == 0 && root.Valid() && root.Kind() == "source_file" && len(n) > 0 && n[0].Kind() == "package_clause" && rangeWithin(root, start, end)
	return selectedRoot{node: root}, ok
}
func allGoDeclarations(ns []parser.Node) bool {
	for _, n := range ns {
		if !isGoDeclaration(n.Kind()) {
			return false
		}
	}
	return true
}

func consumesSnippet(selected, root parser.Node, source string, start, end int) bool {
	if start == end {
		return false
	}
	covered := make([]bool, end-start)
	r := selected.Range()
	for i := max(start, r.StartByte); i < min(end, r.EndByte); i++ {
		covered[i-start] = true
	}
	stack := []parser.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.IsExtra() || n.Kind() == "comment" {
			q := n.Range()
			for i := max(start, q.StartByte); i < min(end, q.EndByte); i++ {
				covered[i-start] = true
			}
			continue
		}
		stack = append(stack, n.Children()...)
	}
	for i, b := range []byte(source[start:end]) {
		if !covered[i] && b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			return false
		}
	}
	return true
}

type slotTarget struct {
	p           generatedPlaceholder
	cardinality SlotCardinality
	kind        string
}

func repeatedSlotsAreScalar(parent parser.Node, placeholders map[spanKey]generatedPlaceholder) bool {
	placeholderCount := 0
	for _, child := range parent.Children() {
		if !child.IsNamed() {
			continue
		}
		r := child.Range()
		if _, ok := placeholders[spanKey{r.StartByte, r.EndByte}]; !ok {
			return false
		}
		placeholderCount++
	}
	return placeholderCount > 1
}

func freezeTemplate(root parser.Node, ps []generatedPlaceholder, maxDepth, clipStart, clipEnd int, sequenceKind string) (templateChild, error) {
	// Index authored occurrences by synthetic span, then choose exactly one typed
	// grammar value per span in a single pre-order traversal.
	bySpan := indexGeneratedPlaceholders(ps)
	targets, err := findSlotTargets(root, bySpan, maxDepth+8)
	if err != nil {
		return templateChild{}, err
	}
	if len(targets) != len(ps) {
		return templateChild{}, fmt.Errorf("not every metavariable occupies a supported Go grammar value")
	}
	builder := templateFreezer{
		targets: targets, maxDepth: maxDepth, clipStart: clipStart, clipEnd: clipEnd,
		sequenceKind: sequenceKind, stack: []templateFreezeFrame{{src: root, depth: 1}},
	}
	return builder.build()
}

func indexGeneratedPlaceholders(ps []generatedPlaceholder) map[spanKey]generatedPlaceholder {
	bySpan := make(map[spanKey]generatedPlaceholder, len(ps))
	for _, p := range ps {
		bySpan[spanKey{p.start, p.end}] = p
	}
	return bySpan
}

type slotTargetWalkItem struct {
	n, parent parser.Node
	depth     int
}

func findSlotTargets(root parser.Node, bySpan map[spanKey]generatedPlaceholder, depthLimit int) (map[spanKey]slotTarget, error) {
	targets := make(map[spanKey]slotTarget, len(bySpan))
	walk := []slotTargetWalkItem{{n: root, depth: 1}}
	for len(walk) > 0 {
		x := walk[len(walk)-1]
		walk = walk[:len(walk)-1]
		if x.depth > depthLimit {
			return nil, fmt.Errorf("Go template exceeds effective depth limit")
		}
		chooseSlotTarget(x.n, x.parent, bySpan, targets)
		walk = appendSlotTargetChildren(walk, x)
	}
	return targets, nil
}

func chooseSlotTarget(n, parent parser.Node, bySpan map[spanKey]generatedPlaceholder, targets map[spanKey]slotTarget) {
	r := n.Range()
	key := spanKey{r.StartByte, r.EndByte}
	p, authored := bySpan[key]
	_, chosen := targets[key]
	if !authored || chosen || !typedPlaceholderNode(n, parent, p.role) {
		return
	}
	cardinality := SlotOne
	if parent.Valid() && repeatedGrammarPosition(parent, n) && !repeatedSlotsAreScalar(parent, bySpan) {
		cardinality = SlotMany
	}
	targets[key] = slotTarget{p: p, cardinality: cardinality, kind: n.Kind()}
}

func appendSlotTargetChildren(walk []slotTargetWalkItem, x slotTargetWalkItem) []slotTargetWalkItem {
	children := x.n.Children()
	for i := len(children) - 1; i >= 0; i-- {
		walk = append(walk, slotTargetWalkItem{n: children[i], parent: x.n, depth: x.depth + 1})
	}
	return walk
}

type templateFreezeFrame struct {
	src      parser.Node
	depth    int
	children []parser.Node
	next     int
	kept     []templateChild
}

type templateFreezer struct {
	targets             map[spanKey]slotTarget
	maxDepth, clipStart int
	clipEnd             int
	sequenceKind        string
	stack               []templateFreezeFrame
}

func (b *templateFreezer) build() (templateChild, error) {
	for len(b.stack) > 0 {
		child, done, err := b.step()
		if err != nil {
			return templateChild{}, err
		}
		if done {
			return child, nil
		}
	}
	return templateChild{}, fmt.Errorf("empty Go template")
}

func (b *templateFreezer) step() (templateChild, bool, error) {
	index := len(b.stack) - 1
	f := &b.stack[index]
	if f.depth > b.maxDepth {
		return templateChild{}, false, fmt.Errorf("Go template exceeds effective depth limit")
	}
	if child, ok := b.slotChild(f.src); ok {
		return b.finishFrame(index, child)
	}
	if f.children == nil {
		f.children = b.keptChildren(f.src)
	}
	if f.next < len(f.children) {
		b.descend(f)
		return templateChild{}, false, nil
	}
	return b.finishFrame(index, b.concreteChild(f, len(b.stack) == 1))
}

func (b *templateFreezer) slotChild(src parser.Node) (templateChild, bool) {
	r := src.Range()
	target, ok := b.targets[spanKey{r.StartByte, r.EndByte}]
	if !ok || src.Kind() != target.kind {
		return templateChild{}, false
	}
	slot := &templateSlot{ref: target.p.ref, cardinality: target.cardinality}
	return templateChild{field: src.FieldName(), slot: slot}, true
}

func (b *templateFreezer) keptChildren(src parser.Node) []parser.Node {
	var children []parser.Node
	for _, child := range src.Children() {
		if !omitTemplateNode(child, b.clipStart, b.clipEnd) {
			children = append(children, child)
		}
	}
	return children
}

func (b *templateFreezer) descend(f *templateFreezeFrame) {
	child := f.children[f.next]
	f.next++
	b.stack = append(b.stack, templateFreezeFrame{src: child, depth: f.depth + 1})
}

func (b *templateFreezer) concreteChild(f *templateFreezeFrame, root bool) templateChild {
	kind := f.src.Kind()
	if root && b.sequenceKind != "" {
		kind = b.sequenceKind
	}
	node := &templateNode{kind: kind, named: f.src.IsNamed(), children: append([]templateChild(nil), f.kept...)}
	if len(f.kept) == 0 {
		node.token = f.src.Text()
	}
	return templateChild{field: f.src.FieldName(), node: node}
}

func (b *templateFreezer) finishFrame(index int, child templateChild) (templateChild, bool, error) {
	b.stack = b.stack[:index]
	if len(b.stack) == 0 {
		return child, true, nil
	}
	parent := &b.stack[len(b.stack)-1]
	parent.kept = append(parent.kept, child)
	return templateChild{}, false, nil
}
func typedPlaceholderNode(n, parent parser.Node, role placeholderRole) bool {
	switch role {
	case roleNode:
		return ordinaryPlaceholderKind(n.Kind())
	case roleImportPath:
		return importPathPlaceholderNode(n, parent)
	case roleDeclarationSpec:
		return declarationSpecPlaceholderNode(n, parent)
	case roleTypeSpec:
		return typeSpecPlaceholderNode(n, parent)
	case roleDeclaration:
		return isGoDeclaration(n.Kind()) && parent.Valid() && parent.Kind() == "source_file"
	default:
		return false
	}
}

func ordinaryPlaceholderKind(kind string) bool {
	switch kind {
	case "identifier", "type_identifier", "field_identifier", "package_identifier", "label_name", "expression_statement", "parameter_declaration", "variadic_parameter_declaration", "field_declaration", "literal_element", "var_spec", "const_spec", "type_spec", "type_elem":
		return true
	default:
		return false
	}
}

func importPathPlaceholderNode(n, parent parser.Node) bool {
	if n.Kind() == "import_spec" {
		return parent.Valid() && parent.Kind() == "import_spec_list"
	}
	return n.Kind() == "interpreted_string_literal" && parent.Valid() && parent.Kind() == "import_spec"
}

func declarationSpecPlaceholderNode(n, parent parser.Node) bool {
	if !parent.Valid() {
		return false
	}
	return n.Kind() == "var_spec" && declarationOrList(parent.Kind(), "var_declaration", "var_spec_list") ||
		n.Kind() == "const_spec" && declarationOrList(parent.Kind(), "const_declaration", "const_spec_list")
}

func typeSpecPlaceholderNode(n, parent parser.Node) bool {
	return parent.Valid() && n.Kind() == "type_spec" && declarationOrList(parent.Kind(), "type_declaration", "type_spec_list")
}

func declarationOrList(kind, declaration, list string) bool {
	return kind == declaration || kind == list
}

// Repeated-position metadata is generated in parser from the pinned grammar's
// node-types.json. Grouped declarations retain syntax-sensitive checks here
// because node-types flattens their grouped alternative.

func repeatedGrammarPosition(parent, child parser.Node) bool {
	p := parent.Kind()
	if child.FieldName() != "" {
		return parser.GrammarFieldCardinality("go", p, child.FieldName()) == parser.GrammarCardinalityMany
	}
	if parser.GrammarChildrenCardinality("go", p) != parser.GrammarCardinalityMany {
		return false
	}
	if p == "const_declaration" || p == "type_declaration" {
		for _, c := range parent.Children() {
			if c.Kind() == "(" {
				return true
			}
		}
		return false
	}
	return true
}

// repeatedGrammarPositionKinds is the snapshot-friendly counterpart used to
// validate generic traversal list targets against the same pinned metadata.
func repeatedGrammarPositionKinds(parentKind, field string, children []parser.SyntaxNode) bool {
	if field != "" {
		return parser.GrammarFieldCardinality("go", parentKind, field) == parser.GrammarCardinalityMany
	}
	if parser.GrammarChildrenCardinality("go", parentKind) != parser.GrammarCardinalityMany {
		return false
	}
	if parentKind == "const_declaration" || parentKind == "type_declaration" {
		for _, child := range children {
			if child.Kind() == "(" {
				return true
			}
		}
		return false
	}
	return true
}
func omitTemplateNode(n parser.Node, start, end int) bool {
	r := n.Range()
	return r.EndByte <= start || r.StartByte >= end || n.IsExtra() || n.Kind() == "comment" || (!n.IsNamed() && r.StartByte == r.EndByte)
}

func restoredOccurrences(t Template, ps []generatedPlaceholder) bool {
	checker := occurrenceRestoration{seen: make([]int, len(ps)), indices: make(map[VariableRef]int, len(ps)), valid: true}
	for _, p := range ps {
		checker.indices[p.ref] = p.index
	}
	if t.rootSlot != nil {
		checker.visitSlot(t.rootSlot)
	}
	checker.visitTree(t.root)
	return checker.valid && checker.seenExactlyOnce()
}

type occurrenceRestoration struct {
	seen    []int
	indices map[VariableRef]int
	valid   bool
}

func (c *occurrenceRestoration) visitSlot(slot *templateSlot) {
	index, ok := c.indices[slot.ref]
	if !ok {
		c.valid = false
		return
	}
	c.seen[index]++
}

func (c *occurrenceRestoration) visitTree(root *templateNode) {
	stack := []*templateNode{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node != nil {
			stack = c.visitChildren(stack, node.children)
		}
	}
}

func (c *occurrenceRestoration) visitChildren(stack []*templateNode, children []templateChild) []*templateNode {
	for i := range children {
		child := &children[i]
		if child.slot != nil {
			c.visitSlot(child.slot)
		} else {
			stack = append(stack, child.node)
		}
	}
	return stack
}

func (c *occurrenceRestoration) seenExactlyOnce() bool {
	for _, count := range c.seen {
		if count != 1 {
			return false
		}
	}
	return true
}
func dedupeTemplates(in []Template) []Template {
	seen := make(map[string]bool, len(in))
	out := make([]Template, 0, len(in))
	for _, t := range in {
		k := canonicalTemplate(t)
		if !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	return out
}
func canonicalTemplate(t Template) string {
	var b strings.Builder
	b.WriteByte(byte(t.context))
	slot := func(s *templateSlot) {
		b.WriteByte('$')
		b.WriteString(strconv.FormatUint(uint64(s.ref.ID), 10))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(s.ref.Range.StartByte))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(s.ref.Range.EndByte))
		b.WriteByte(byte(s.cardinality))
	}
	if t.rootSlot != nil {
		slot(t.rootSlot)
		return b.String()
	}
	type event struct {
		n     *templateNode
		c     *templateChild
		close bool
	}
	stack := []event{{n: t.root}}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if x.close {
			b.WriteByte(')')
			continue
		}
		if x.c != nil {
			b.WriteByte('[')
			b.WriteString(x.c.field)
			b.WriteByte(']')
			if x.c.slot != nil {
				slot(x.c.slot)
			} else {
				stack = append(stack, event{n: x.c.node})
			}
			continue
		}
		if x.n == nil {
			continue
		}
		b.WriteByte('(')
		b.WriteString(x.n.kind)
		b.WriteByte('|')
		b.WriteString(x.n.token)
		b.WriteByte('|')
		stack = append(stack, event{close: true})
		for i := len(x.n.children) - 1; i >= 0; i-- {
			stack = append(stack, event{c: &x.n.children[i]})
		}
	}
	return b.String()
}
