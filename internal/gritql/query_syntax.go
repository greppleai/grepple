package gritql

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/greppleai/grepple/internal/gritql/internal/gritgrammar"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type queryErrorKind uint8

const (
	queryMalformed queryErrorKind = iota + 1
	queryUnsupported
	queryInvalidContext
	queryDepthLimit
)

type queryPosition struct {
	Line   int
	Column int
}

type queryRange struct {
	StartByte int
	EndByte   int
	Start     queryPosition
	End       queryPosition
}

type querySyntaxError struct {
	Kind    queryErrorKind
	Message string
	Range   queryRange
}

func (e *querySyntaxError) Error() string { return e.Message }

type queryParseDiagnostic struct {
	Kind    string
	Message string
	Range   queryRange
}

// noCopy makes accidental value copies of native tree ownership visible to go vet.
type noCopy struct{}

func (*noCopy) Lock() {}

// queryDocument owns both the immutable query bytes and its tree-sitter tree.
// It must not be copied. Its nodes are invalidated by close.
type queryDocument struct {
	noCopy     noCopy
	mu         sync.RWMutex
	source     []byte
	lineStarts []int
	tree       *sitter.Tree
}

type queryNode struct {
	doc   *queryDocument
	raw   *sitter.Node
	field string
}

// parseQuery always uses the pinned generated GritQL parser. The closed-v1
// validation is an allow-list over that concrete tree/input; unsupported input
// retains the upstream tree so the compiler can produce feature diagnostics.
func parseQuery(source []byte) (*queryDocument, error) {
	return parseQueryWithMaxDepth(source, hardMaxDepth)
}

func parseQueryWithMaxDepth(source []byte, maxDepth int) (*queryDocument, error) {
	if err := validQueryEncoding(source); err != nil {
		return nil, err
	}
	owned := bytes.Clone(source)
	tree, err := parsePinnedQuery(parserInput(owned))
	if err != nil {
		return nil, err
	}
	doc := &queryDocument{source: owned, lineStarts: lineStarts(owned), tree: tree}
	runtime.SetFinalizer(doc, func(d *queryDocument) { d.close() })

	classification := validateV1Tree(doc, maxDepth)
	if classification == queryMalformed && doc.hasUnsupportedConstruct() {
		classification = queryUnsupported
	}
	if classification != 0 {
		return doc, &querySyntaxError{
			Kind: classification, Message: queryClassificationMessage(classification, queryCompatibility(doc)),
			Range: doc.rangeForBytes(0, len(owned)),
		}
	}
	return doc, nil
}

func validQueryEncoding(source []byte) error {
	if utf8.Valid(source) {
		return nil
	}
	whole := rangeInBytes(source, lineStarts(source), 0, len(source))
	return &querySyntaxError{Kind: queryMalformed, Message: "pattern is not valid UTF-8", Range: whole}
}

func parserInput(owned []byte) []byte {
	if bytes.IndexByte(owned, 0) < 0 {
		return owned
	}
	// tree-sitter reserves NUL as its end-of-input sentinel. Parse an
	// equal-width surrogate and validate every original lexical leaf below.
	input := bytes.Clone(owned)
	for i, b := range input {
		if b == 0 {
			input[i] = 'x'
		}
	}
	return input
}

func parsePinnedQuery(source []byte) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	language := sitter.NewLanguage(gritgrammar.Language())
	if err := parser.SetLanguage(language); err != nil {
		parser.Close()
		return nil, fmt.Errorf("configure pinned GritQL parser: %w", err)
	}
	tree := parser.Parse(source, nil)
	parser.Close()
	if tree == nil {
		return nil, fmt.Errorf("pinned GritQL parser returned no tree")
	}
	return tree, nil
}

func queryClassificationMessage(classification queryErrorKind, compatibility string) string {
	switch classification {
	case queryDepthLimit:
		return "pattern exceeds parse depth"
	case queryUnsupported:
		return "construct is outside " + compatibility
	case queryInvalidContext:
		return "anonymous wildcard is invalid on a constraint left side"
	default:
		return "pattern does not satisfy " + compatibility
	}
}

func queryCompatibility(*queryDocument) string { return Compatibility }

func (d *queryDocument) close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tree == nil {
		return
	}
	d.tree.Close()
	d.tree = nil
	d.source = nil
	d.lineStarts = nil
	runtime.SetFinalizer(d, nil)
}

func (d *queryDocument) bytes() []byte {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return nil
	}
	return bytes.Clone(d.source)
}

func (d *queryDocument) root() queryNode {
	if d == nil {
		return queryNode{}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return queryNode{}
	}
	return queryNode{doc: d, raw: d.tree.RootNode()}
}

func (d *queryDocument) rangeForBytes(start, end int) queryRange {
	if d == nil {
		return queryRange{}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return queryRange{}
	}
	return rangeInBytes(d.source, d.lineStarts, start, end)
}

func (d *queryDocument) diagnostics() []queryParseDiagnostic {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return nil
	}
	var out []queryParseDiagnostic
	stack := []*sitter.Node{d.tree.RootNode()}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.IsError() || n.IsMissing() {
			message := "syntax error"
			if n.IsMissing() {
				message = "missing " + n.Kind()
			}
			out = append(out, queryParseDiagnostic{Kind: n.Kind(), Message: message, Range: rangeInBytes(d.source, d.lineStarts, int(n.StartByte()), int(n.EndByte()))})
		}
		for i := n.ChildCount(); i > 0; i-- {
			if child := n.Child(i - 1); child != nil {
				stack = append(stack, child)
			}
		}
	}
	return out
}

var supportedV1SyntaxKinds = map[string]bool{
	"source_file": true, "comment": true, "langdecl": true, "languageName": true, "name": true,
	"codeSnippet": true, "backtickSnippet": true, "nodeLike": true, "namedArg": true,
	"regexPattern": true, "regex": true, "variable": true, "emptyPredicate": true,
	"patternAnd": true, "patternOr": true, "patternNot": true,
	"patternMaybe": true, "patternContains": true, "patternParent": true, "within": true,
	"patternWhere": true, "patternAs": true, "predicateAnd": true, "predicateMatch": true,
}

var unsupportedErrorNames = map[string]bool{
	"import": true, "module": true, "library": true, "remote": true,
	"multifile": true, "sequential": true, "files": true, "global": true,
	"pattern": true, "predicate": true, "function": true, "foreign": true,
}

func (d *queryDocument) hasUnsupportedConstruct() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return false
	}
	root := d.tree.RootNode()
	language := root.ChildByFieldName("language")
	hasLanguageDeclaration := language != nil && language.Kind() == "langdecl"
	stack := []*sitter.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if unsupportedSyntaxNode(d.source, n, hasLanguageDeclaration) {
			return true
		}
		for i := n.ChildCount(); i > 0; i-- {
			if child := n.Child(i - 1); child != nil {
				stack = append(stack, child)
			}
		}
	}
	return false
}

func unsupportedSyntaxNode(source []byte, node *sitter.Node, hasLanguageDeclaration bool) bool {
	if node.IsNamed() && !node.IsError() && !node.IsMissing() && !supportedV1SyntaxKinds[node.Kind()] {
		return true
	}
	// Constructor patterns with snippets remain outside this subset. Node-like
	// selectors accept only structural nested nodes and metavariable captures.
	if node.Kind() == "namedArg" {
		if pattern := node.ChildByFieldName("pattern"); pattern != nil && pattern.Kind() != "nodeLike" && pattern.Kind() != "variable" {
			return true
		}
	}
	if node.Kind() == "patternAs" {
		pattern := node.ChildByFieldName("pattern")
		if pattern == nil || pattern.Kind() != "nodeLike" {
			return true
		}
	}
	text := source[int(node.StartByte()):int(node.EndByte())]
	if node.Kind() == "comment" && bytes.HasPrefix(text, []byte("/*")) {
		return true
	}
	if node.IsError() && !hasLanguageDeclaration && bytes.HasPrefix(bytes.TrimSpace(text), []byte("language ")) {
		return true
	}
	parent := node.Parent()
	if unsupportedLanguageName(source, node, parent, hasLanguageDeclaration) {
		return true
	}
	return unsupportedErrorChild(text, node, parent)
}

func unsupportedLanguageName(source []byte, node, parent *sitter.Node, hasLanguageDeclaration bool) bool {
	if node.Kind() != "languageName" || parent != nil && parent.Kind() == "langdecl" {
		return false
	}
	if parent == nil || !hasLanguageDeclaration {
		return true
	}
	parentText := source[int(parent.StartByte()):int(parent.EndByte())]
	return !bytes.HasPrefix(bytes.TrimSpace(parentText), []byte("language "))
}

func unsupportedErrorChild(text []byte, node, parent *sitter.Node) bool {
	if parent == nil || !parent.IsError() {
		return false
	}
	if node.Kind() == "name" && unsupportedErrorNames[string(text)] {
		return true
	}
	return !node.IsNamed() && bytes.ContainsAny(text, ";()[]=!%@#^&|+*/")
}

func (n queryNode) read(fn func()) {
	if n.doc == nil || n.raw == nil {
		return
	}
	n.doc.mu.RLock()
	defer n.doc.mu.RUnlock()
	if n.doc.tree != nil {
		fn()
	}
}

func (n queryNode) valid() bool {
	valid := false
	n.read(func() { valid = true })
	return valid
}

func (n queryNode) kind() string {
	var value string
	n.read(func() { value = n.raw.Kind() })
	return value
}

func (n queryNode) named() bool {
	var value bool
	n.read(func() { value = n.raw.IsNamed() })
	return value
}

func (n queryNode) extra() bool {
	var value bool
	n.read(func() { value = n.raw.IsExtra() })
	return value
}

func (n queryNode) error() bool {
	var value bool
	n.read(func() { value = n.raw.IsError() })
	return value
}

func (n queryNode) missing() bool {
	var value bool
	n.read(func() { value = n.raw.IsMissing() })
	return value
}

func (n queryNode) fieldName() string { return n.field }

func (n queryNode) text() string {
	var value string
	n.read(func() {
		start, end := int(n.raw.StartByte()), int(n.raw.EndByte())
		if start <= end && end <= len(n.doc.source) {
			value = string(n.doc.source[start:end])
		}
	})
	return value
}

func (n queryNode) byteRange() queryRange {
	var value queryRange
	n.read(func() {
		value = rangeInBytes(n.doc.source, n.doc.lineStarts, int(n.raw.StartByte()), int(n.raw.EndByte()))
	})
	return value
}

func (n queryNode) childByFieldName(name string) queryNode {
	var value queryNode
	n.read(func() {
		if child := n.raw.ChildByFieldName(name); child != nil {
			value = queryNode{doc: n.doc, raw: child, field: name}
		}
	})
	return value
}

func (n queryNode) children() []queryNode {
	var result []queryNode
	n.read(func() {
		result = make([]queryNode, 0, n.raw.ChildCount())
		for i := uint(0); i < n.raw.ChildCount(); i++ {
			if child := n.raw.Child(i); child != nil {
				result = append(result, queryNode{doc: n.doc, raw: child, field: n.raw.FieldNameForChild(uint32(i))})
			}
		}
	})
	return result
}

func (n queryNode) namedChildren() []queryNode {
	var result []queryNode
	n.read(func() {
		result = make([]queryNode, 0, n.raw.NamedChildCount())
		for i := uint(0); i < n.raw.NamedChildCount(); i++ {
			if child := n.raw.NamedChild(i); child != nil {
				result = append(result, queryNode{doc: n.doc, raw: child, field: n.raw.FieldNameForNamedChild(uint32(i))})
			}
		}
	})
	return result
}

func lineStarts(source []byte) []int {
	starts := []int{0}
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func rangeInBytes(source []byte, starts []int, start, end int) queryRange {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > len(source) {
		end = len(source)
	}
	position := func(offset int) queryPosition {
		line := 0
		for line+1 < len(starts) && starts[line+1] <= offset {
			line++
		}
		columnBytes := source[starts[line]:offset]
		// RuneCount treats each malformed byte as one RuneError, making even an
		// invalid-UTF-8 whole-query diagnostic deterministic.
		return queryPosition{Line: line + 1, Column: utf8.RuneCount(columnBytes) + 1}
	}
	return queryRange{StartByte: start, EndByte: end, Start: position(start), End: position(end)}
}

// validateV1Tree is the closed gritql-v1 allow-list over the pinned parser's
// concrete tree. It validates each node locally and never constructs a second AST.
func validateV1Tree(doc *queryDocument, maxDepth int) queryErrorKind {
	if maxDepth <= 0 || maxDepth > hardMaxDepth {
		maxDepth = hardMaxDepth
	}
	root := doc.root()
	if classification := validateV1Envelope(doc, root); classification != 0 {
		return classification
	}
	if !validV1Leaves(doc.source, root) {
		return queryMalformed
	}
	return validateV1Structure(root, maxDepth)
}

func validateV1Envelope(doc *queryDocument, root queryNode) queryErrorKind {
	if !root.valid() || root.kind() != "source_file" || !root.named() || root.extra() || root.fieldName() != "" {
		return queryMalformed
	}
	if len(doc.diagnostics()) != 0 || hasBareCarriageReturn(doc.source) {
		return queryMalformed
	}
	children := withoutComments(root.children())
	if len(children) != 2 || !isNode(children[0], "langdecl", "language", true) || !children[1].named() || children[1].fieldName() != "pattern" {
		return queryMalformed
	}
	language, pattern := children[0], children[1]
	if !validEnvelopeSpacing(doc.source, language, pattern) {
		return queryMalformed
	}
	if _, ok := targetLanguageByID(language.childByFieldName("name").text()); !ok {
		return queryUnsupported
	}
	return 0
}

func hasBareCarriageReturn(source []byte) bool {
	for i, b := range source {
		if b == '\r' && (i+1 >= len(source) || source[i+1] != '\n') {
			return true
		}
	}
	return false
}

func validV1Leaves(source []byte, root queryNode) bool {
	previousLeafEnd := 0
	// An ordered concrete traversal validates that bytes omitted as extras are
	// only v1 whitespace, and that every lexical leaf is closed-v1.
	stack := []queryNode{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		kids := n.children()
		if len(kids) == 0 {
			r := n.byteRange()
			if r.StartByte < previousLeafEnd || !validSpacing(source[previousLeafEnd:r.StartByte]) || !validV1Leaf(n) {
				return false
			}
			previousLeafEnd = r.EndByte
			continue
		}
		for i := len(kids) - 1; i >= 0; i-- {
			stack = append(stack, kids[i])
		}
	}
	return validSpacing(source[previousLeafEnd:])
}

type v1ValidationWork struct {
	node  queryNode
	depth int
}

func validateV1Structure(root queryNode, maxDepth int) queryErrorKind {
	stack := []v1ValidationWork{{root, 0}}
	invalidContext := false
	for len(stack) > 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		classification, anonymousLeft, children := validateV1WorkItem(item, maxDepth)
		if classification != 0 {
			return classification
		}
		invalidContext = invalidContext || anonymousLeft
		stack = append(stack, children...)
	}
	if invalidContext {
		return queryInvalidContext
	}
	return 0
}

func validateV1WorkItem(item v1ValidationWork, maxDepth int) (queryErrorKind, bool, []v1ValidationWork) {
	node := item.node
	if !node.valid() || node.error() || node.missing() {
		return queryMalformed, false, nil
	}
	if node.kind() == "comment" {
		if !validV1CommentNode(node) {
			return queryMalformed, false, nil
		}
		return 0, false, nil
	}
	if isV1Expression(node.kind()) && item.depth > maxDepth {
		return queryDepthLimit, false, nil
	}
	ok, anonymousLeft := validV1Node(node)
	if !ok {
		return queryMalformed, false, nil
	}
	return 0, anonymousLeft, v1ChildWork(node, item.depth)
}

func v1ChildWork(parent queryNode, parentDepth int) []v1ValidationWork {
	children := parent.children()
	work := make([]v1ValidationWork, 0, len(children))
	for _, child := range children {
		work = append(work, v1ValidationWork{child, childV1Depth(parent, child, parentDepth)})
	}
	return work
}

func validV1CommentNode(node queryNode) bool {
	return node.named() && node.extra() && node.fieldName() == "" && len(node.children()) == 0
}

func childV1Depth(parent, child queryNode, parentDepth int) int {
	if !isV1Expression(child.kind()) {
		return parentDepth
	}
	if isV1Expression(parent.kind()) {
		return parentDepth + 1
	}
	if parent.kind() == "source_file" || parent.kind() == "predicateMatch" {
		return parentDepth + 1
	}
	return parentDepth
}

func validEnvelopeSpacing(source []byte, language, pattern queryNode) bool {
	languageRange, patternRange := language.byteRange(), pattern.byteRange()
	leadingOK, _ := validTrivia(source[:languageRange.StartByte])
	trailingOK, _ := validTrivia(source[patternRange.EndByte:])
	gapOK, hasNewline := validTrivia(source[languageRange.EndByte:patternRange.StartByte])
	return leadingOK && trailingOK && gapOK && hasNewline
}

func validTrivia(value []byte) (bool, bool) {
	hasNewline := false
	for pos := 0; pos < len(value); {
		switch value[pos] {
		case ' ', '\t':
			pos++
		case '\n':
			hasNewline = true
			pos++
		case '\r':
			if pos+1 >= len(value) || value[pos+1] != '\n' {
				return false, hasNewline
			}
			hasNewline = true
			pos += 2
		case '/':
			if pos+1 >= len(value) || value[pos+1] != '/' {
				return false, hasNewline
			}
			pos += 2
			for pos < len(value) && value[pos] != '\r' && value[pos] != '\n' {
				pos++
			}
		default:
			return false, hasNewline
		}
	}
	return true, hasNewline
}

type v1NodeValidator func(queryNode, []queryNode) bool

var v1NodeValidators = map[string]v1NodeValidator{
	"nodeLike":     validNodeLikeNode,
	"namedArg":     validNodeArgument,
	"name":         validNamedTerminalNode,
	"source_file":  validSourceFileNode,
	"langdecl":     validLanguageDeclarationNode,
	"languageName": validLanguageNameNode,
	"codeSnippet": func(n queryNode, children []queryNode) bool {
		return validWrappedLeafNode(n, children, "backtickSnippet", "source")
	},
	"regexPattern": func(n queryNode, children []queryNode) bool {
		return validWrappedLeafNode(n, children, "regex", "regex")
	},
	"patternAnd": func(n queryNode, children []queryNode) bool { return validBlockNode(n, children, "and") },
	"patternOr":  func(n queryNode, children []queryNode) bool { return validBlockNode(n, children, "or") },
	"patternNot": func(n queryNode, children []queryNode) bool {
		return validUnaryNode(n, children, "not", "pattern")
	},
	"patternMaybe": func(n queryNode, children []queryNode) bool {
		return validUnaryNode(n, children, "maybe", "pattern")
	},
	"patternContains": func(n queryNode, children []queryNode) bool {
		return validUnaryNode(n, children, "contains", "contains")
	},
	"within": validWithinNode,
	"patternParent": func(n queryNode, children []queryNode) bool {
		return n.named() && len(children) == 5 && isNode(children[0], "parent", "", false) && isNode(children[1], "kind", "", false) && isNode(children[2], "(", "", false) && isNode(children[3], "\"function\"", "", false) && isNode(children[4], ")", "", false) && children[0].byteRange().EndByte < children[1].byteRange().StartByte
	},
	"emptyPredicate": func(n queryNode, children []queryNode) bool {
		return n.named() && len(children) == 0 && n.text() == "empty"
	},
	"patternWhere":    validWhereNode,
	"patternAs":       validAsNode,
	"predicateAnd":    validPredicateBlock,
	"predicateMatch":  validPredicateMatchNode,
	"comment":         validCommentNode,
	"backtickSnippet": validNamedTerminalNode,
	"regex":           validNamedTerminalNode,
	"variable":        validNamedTerminalNode,
	"language":        validAnonymousTerminalNode,
	"c":               validAnonymousTerminalNode,
	"cpp":             validAnonymousTerminalNode,
	"csharp":          validAnonymousTerminalNode,
	"dart":            validAnonymousTerminalNode,
	"hcl":             validAnonymousTerminalNode,
	"go":              validAnonymousTerminalNode,
	"java":            validAnonymousTerminalNode,
	"javascript":      validAnonymousTerminalNode,
	"kotlin":          validAnonymousTerminalNode,
	"php":             validAnonymousTerminalNode,
	"python":          validAnonymousTerminalNode,
	"rust":            validAnonymousTerminalNode,
	"shell":           validAnonymousTerminalNode,
	"swift":           validAnonymousTerminalNode,
	"typescript":      validAnonymousTerminalNode,
	"tsx":             validAnonymousTerminalNode,
	"and":             validAnonymousTerminalNode,
	"or":              validAnonymousTerminalNode,
	"not":             validAnonymousTerminalNode,
	"parent":          validAnonymousTerminalNode,
	"kind":            validAnonymousTerminalNode,
	"=":               validAnonymousTerminalNode,
	"(":               validAnonymousTerminalNode,
	")":               validAnonymousTerminalNode,
	"\"function\"":    validAnonymousTerminalNode,
	"maybe":           validAnonymousTerminalNode,
	"contains":        validAnonymousTerminalNode,
	"where":           validAnonymousTerminalNode,
	"as":              validAnonymousTerminalNode,
	"{":               validAnonymousTerminalNode,
	"}":               validAnonymousTerminalNode,
	"<:":              validAnonymousTerminalNode,
	",":               validCommaNode,
}

func validV1Node(node queryNode) (bool, bool) {
	if node.kind() != "comment" && node.extra() {
		return false, false
	}
	validator, supported := v1NodeValidators[node.kind()]
	if !supported {
		return false, false
	}
	children := withoutComments(node.children())
	valid := validator(node, children)
	return valid, valid && node.kind() == "predicateMatch" && children[0].text() == "$_"
}

func validSourceFileNode(node queryNode, children []queryNode) bool {
	return node.named() && node.fieldName() == "" && len(children) == 2 && isNode(children[0], "langdecl", "language", true) && children[1].named() && children[1].fieldName() == "pattern"
}

func validLanguageDeclarationNode(node queryNode, children []queryNode) bool {
	if !node.named() || len(children) != 2 || !isNode(children[0], "language", "", false) || !isNode(children[1], "languageName", "name", true) {
		return false
	}
	a, b := children[0].byteRange(), children[1].byteRange()
	gap := node.doc.source[a.EndByte:b.StartByte]
	return len(gap) > 0 && bytes.IndexFunc(gap, func(r rune) bool { return r != ' ' && r != '\t' }) < 0
}

func validLanguageNameNode(node queryNode, children []queryNode) bool {
	return node.named() && node.fieldName() == "name" && len(children) == 1 && isNode(children[0], node.text(), "", false) && len(children[0].children()) == 0
}

func validWrappedLeafNode(node queryNode, children []queryNode, kind, field string) bool {
	return node.named() && len(children) == 1 && isNode(children[0], kind, field, true) && len(children[0].children()) == 0
}

func validWithinNode(node queryNode, children []queryNode) bool {
	if !node.named() {
		return node.fieldName() == "" && len(children) == 0
	}
	return validUnaryNode(node, children, "within", "pattern")
}

func validPredicateMatchNode(node queryNode, children []queryNode) bool {
	return node.named() && len(children) == 3 && isNode(children[0], "variable", "left", true) && isNode(children[1], "<:", "", false) && children[2].named() && children[2].fieldName() == "right"
}

func validCommentNode(node queryNode, children []queryNode) bool {
	return node.named() && node.extra() && node.fieldName() == "" && len(children) == 0
}

func validNamedTerminalNode(node queryNode, children []queryNode) bool {
	return node.named() && len(children) == 0
}

func validAnonymousTerminalNode(node queryNode, children []queryNode) bool {
	return !node.named() && node.fieldName() == "" && len(children) == 0
}

func validCommaNode(node queryNode, children []queryNode) bool {
	return !node.named() && (node.fieldName() == "patterns" || node.fieldName() == "predicates" || node.fieldName() == "named_args") && len(children) == 0
}

func validBlockNode(node queryNode, concrete []queryNode, keyword string) bool {
	if !node.named() || len(concrete) < 6 || !isNode(concrete[0], keyword, "", false) || !isNode(concrete[1], "{", "", false) || !isNode(concrete[len(concrete)-1], "}", "", false) {
		return false
	}
	if concrete[0].byteRange().EndByte == concrete[1].byteRange().StartByte {
		return false
	}
	count, expectPattern := 0, true
	for _, child := range concrete[2 : len(concrete)-1] {
		if expectPattern {
			if !child.named() || child.fieldName() != "patterns" || !isV1Expression(child.kind()) {
				return false
			}
			count++
		} else if !isNode(child, ",", "patterns", false) {
			return false
		}
		expectPattern = !expectPattern
	}
	return count >= 2
}

func validUnaryNode(node queryNode, concrete []queryNode, keyword, field string) bool {
	return node.named() && len(concrete) == 2 && isNode(concrete[0], keyword, "", false) && concrete[1].named() && concrete[1].fieldName() == field && isV1Expression(concrete[1].kind()) && concrete[0].byteRange().EndByte < concrete[1].byteRange().StartByte
}

// Only node-like selectors may capture the exact matched root in gritql-v1.
func validAsNode(node queryNode, concrete []queryNode) bool {
	return node.named() && len(concrete) == 3 && isNode(concrete[0], "nodeLike", "pattern", true) &&
		isNode(concrete[1], "as", "", false) && isNode(concrete[2], "variable", "variable", true) &&
		concrete[2].text() != "$_" && concrete[0].byteRange().EndByte < concrete[1].byteRange().StartByte &&
		concrete[1].byteRange().EndByte < concrete[2].byteRange().StartByte
}

func validWhereNode(node queryNode, concrete []queryNode) bool {
	return node.named() && len(concrete) == 3 && concrete[0].named() && concrete[0].kind() != "patternWhere" && concrete[0].fieldName() == "pattern" && isV1Expression(concrete[0].kind()) && isNode(concrete[1], "where", "", false) && isNode(concrete[2], "predicateAnd", "side_condition", true) && concrete[1].byteRange().EndByte < concrete[2].byteRange().StartByte
}

func validPredicateBlock(node queryNode, concrete []queryNode) bool {
	if !node.named() || len(concrete) < 3 || !isNode(concrete[0], "{", "", false) || !isNode(concrete[len(concrete)-1], "}", "", false) {
		return false
	}
	count, expectPredicate := 0, true
	for _, child := range concrete[1 : len(concrete)-1] {
		if expectPredicate {
			if !isNode(child, "predicateMatch", "predicates", true) {
				return false
			}
			count++
		} else if !isNode(child, ",", "predicates", false) {
			return false
		}
		expectPredicate = !expectPredicate
	}
	return count > 0
}

func validNodeLikeNode(node queryNode, concrete []queryNode) bool {
	if !node.named() || len(concrete) < 3 || !isNode(concrete[0], "name", "name", true) || !isNode(concrete[1], "(", "", false) || !isNode(concrete[len(concrete)-1], ")", "", false) {
		return false
	}
	expectArg := true
	for _, child := range concrete[2 : len(concrete)-1] {
		if expectArg && !isNode(child, "namedArg", "named_args", true) || !expectArg && !isNode(child, ",", "named_args", false) {
			return false
		}
		expectArg = !expectArg
	}
	return len(concrete) == 3 || !expectArg
}

func validNodeArgument(node queryNode, concrete []queryNode) bool {
	if !node.named() || node.fieldName() != "named_args" {
		return false
	}
	if len(concrete) == 1 {
		return concrete[0].named() && concrete[0].fieldName() == "variable" && (concrete[0].kind() == "variable" || concrete[0].kind() == "nodeLike")
	}
	return len(concrete) == 3 && isNode(concrete[0], "name", "name", true) && isNode(concrete[1], "=", "", false) && concrete[2].named() && concrete[2].fieldName() == "pattern" && (concrete[2].kind() == "variable" || concrete[2].kind() == "nodeLike")
}
func isNode(node queryNode, kind, field string, named bool) bool {
	return node.valid() && node.kind() == kind && node.fieldName() == field && node.named() == named && !node.error() && !node.missing()
}

func withoutComments(children []queryNode) []queryNode {
	out := make([]queryNode, 0, len(children))
	for _, child := range children {
		if child.kind() != "comment" {
			out = append(out, child)
		}
	}
	return out
}

func isV1Expression(kind string) bool {
	switch kind {
	case "codeSnippet", "nodeLike", "patternAs", "regexPattern", "emptyPredicate", "patternParent", "patternAnd", "patternOr", "patternNot", "patternMaybe", "patternContains", "within", "patternWhere":
		return true
	default:
		return false
	}
}

func validV1Leaf(node queryNode) bool {
	if len(node.children()) != 0 {
		return false
	}
	switch node.kind() {
	case "comment":
		text := []byte(node.text())
		return node.named() && node.extra() && node.fieldName() == "" && len(text) >= 2 && text[0] == '/' && text[1] == '/' && bytes.IndexByte(text, '\n') < 0
	case "backtickSnippet":
		return validSnippetLeaf([]byte(node.text()))
	case "regex":
		return validRegexLeaf([]byte(node.text()))
	case "variable":
		return validMetavariable([]byte(node.text()))
	default:
		text := []byte(node.text())
		return bytes.IndexByte(text, 0) < 0 && !bytes.Contains(text, []byte{0xef, 0xbb, 0xbf})
	}
}

func validSnippetLeaf(payload []byte) bool {
	if len(payload) < 2 || payload[0] != '`' || payload[len(payload)-1] != '`' {
		return false
	}
	for pos := 1; pos < len(payload)-1; {
		switch payload[pos] {
		case '\\':
			if pos+1 >= len(payload)-1 || payload[pos+1] != '`' && payload[pos+1] != '\\' {
				return false
			}
			pos += 2
		case '$':
			end := metavariableEnd(payload, pos, len(payload)-1)
			if end < 0 {
				return false
			}
			pos = end
		default:
			pos++
		}
	}
	return true
}

func validRegexLeaf(payload []byte) bool {
	if len(payload) < 3 || payload[0] != 'r' || payload[1] != '"' || payload[len(payload)-1] != '"' {
		return false
	}
	for pos, limit := 2, len(payload)-1; pos < limit; {
		if payload[pos] == '\n' || payload[pos] == '\r' {
			return false
		}
		if payload[pos] != '\\' {
			pos++
			continue
		}
		pos = regexEscapeEnd(payload, pos, limit)
		if pos < 0 {
			return false
		}
	}
	return true
}

func regexEscapeEnd(payload []byte, pos, limit int) int {
	pos++
	if pos >= limit {
		return -1
	}
	if strings.ContainsRune("\"\\nrt", rune(payload[pos])) {
		return pos + 1
	}
	digits := 0
	if payload[pos] == 'x' {
		digits = 2
	} else if payload[pos] == 'u' {
		digits = 4
	} else {
		return -1
	}
	pos++
	if pos+digits > limit {
		return -1
	}
	for _, b := range payload[pos : pos+digits] {
		if !isHex(b) {
			return -1
		}
	}
	return pos + digits
}

func validMetavariable(value []byte) bool {
	return metavariableEnd(value, 0, len(value)) == len(value)
}

func metavariableEnd(value []byte, start, limit int) int {
	if start >= limit || value[start] != '$' || start+1 >= limit {
		return -1
	}
	pos := start + 1
	if value[pos] == '_' {
		pos++
		if pos < limit && isMetaContinue(value[pos]) {
			return -1
		}
		return pos
	}
	if !isASCIIAlpha(value[pos]) {
		return -1
	}
	pos++
	for pos < limit && isMetaContinue(value[pos]) {
		pos++
	}
	return pos
}

func validSpacing(value []byte) bool {
	for pos := 0; pos < len(value); {
		switch value[pos] {
		case ' ', '\t', '\n':
			pos++
		case '\r':
			if pos+1 >= len(value) || value[pos+1] != '\n' {
				return false
			}
			pos += 2
		default:
			return false
		}
	}
	return true
}

func isASCIIAlpha(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
func isMetaContinue(b byte) bool {
	return isASCIIAlpha(b) || b >= '0' && b <= '9' || b == '_'
}
func isHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}
