package gritql

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strconv"
	"unicode/utf8"

	"github.com/greppleai/grepple/internal/parser"
)

// Compatibility identifies the unified native GritQL language contract.
const Compatibility = "gritql-v1"

// MultilingualCompatibility is retained as a source-compatible alias.
// Deprecated: use Compatibility.
const MultilingualCompatibility = Compatibility

// CompatibilityVersion is an explicit alias for metadata producers.
const CompatibilityVersion = Compatibility

// Position and Range identify a half-open range in the submitted query.
type Position struct {
	Line   int
	Column int
}

// Range identifies a half-open byte and source-position interval in a query.
type Range struct {
	StartByte int
	EndByte   int
	Start     Position
	End       Position
}

// CompileOptions bounds work done while compiling. Zero values select the v1 defaults.
type CompileOptions struct {
	MaxPatternBytes      int
	MaxRegexBytes        int
	MaxRegexInstructions int
	MaxDepth             int
}

const (
	defaultMaxPatternBytes      = 256 << 10
	hardMaxPatternBytes         = 1 << 20
	defaultMaxRegexBytes        = 16 << 10
	hardMaxRegexBytes           = 64 << 10
	defaultMaxRegexInstructions = 100_000
	hardMaxRegexInstructions    = 500_000
	defaultMaxDepth             = 256
	hardMaxDepth                = 1024
)

// CompileError is a stable compile diagnostic. Resource depth errors have no range.
type CompileError struct {
	Code    string
	Class   string
	Message string
	Range   *Range
	cause   error
}

func (e *CompileError) Error() string { return e.Message }
func (e *CompileError) Unwrap() error { return e.cause }

// VariableID is assigned by the first source occurrence of a named metavariable.
// Zero is reserved for the anonymous wildcard $_.
type VariableID uint32

// Variable describes a distinct named metavariable and its first source occurrence.
type Variable struct {
	ID    VariableID
	Name  string
	Range Range
}

// VariableRef identifies one metavariable occurrence in a compiled expression.
type VariableRef struct {
	ID        VariableID
	Name      string
	Anonymous bool
	Range     Range
}

// Kind is an immutable IR operation.
type Kind uint8

// KindSnippet and the following constants identify supported immutable IR operations.
const (
	KindSnippet Kind = iota + 1
	KindRegex
	KindAnd
	KindOr
	KindNot
	KindMaybe
	KindContains
	KindWithin
	KindWhere
	KindEmpty
	KindParent
	KindNodeLike
	KindNodeCapture
	KindAs
)

func (k Kind) String() string {
	names := [...]string{"", "snippet", "regex", "and", "or", "not", "maybe", "contains", "within", "where", "empty", "parent", "node", "capture", "as"}
	if int(k) >= len(names) {
		return fmt.Sprintf("Kind(%d)", k)
	}
	return names[k]
}

// FeatureSet summarizes syntax used by a Program.
type FeatureSet uint32

// FeatureSnippet and the following flags identify syntax present in a compiled program.
const (
	FeatureSnippet FeatureSet = 1 << iota
	FeatureRegex
	FeatureAnd
	FeatureOr
	FeatureNot
	FeatureMaybe
	FeatureContains
	FeatureWithin
	FeatureWhere
	FeatureVariables
	FeatureEmpty
	FeatureParent
	FeatureNodeLike
	FeatureAs
)

// Has reports whether every requested feature is present in the set.
func (f FeatureSet) Has(feature FeatureSet) bool { return f&feature == feature }

type expression struct {
	kind        Kind
	rng         Range
	text        string
	refs        []VariableRef
	children    []*expression
	fields      []string // node field names; empty means a positional named child
	constraints []constraint
	re          *regexp.Regexp
	template    Template
	templates   []Template
}

type constraint struct {
	lhs VariableRef
	rhs *expression
	rng Range
}

// Expression is a read-only handle into a Program. Its accessors return copies.
type Expression struct{ e *expression }

// Kind returns the expression's IR operation, or zero for an empty handle.
func (e Expression) Kind() Kind {
	if e.e == nil {
		return 0
	}
	return e.e.kind
}

// Range returns the expression's query-source range, or an empty range for an empty handle.
func (e Expression) Range() Range {
	if e.e == nil {
		return Range{}
	}
	return e.e.rng
}

// Text is the exactly decoded snippet or regular expression. It is empty for operators.
func (e Expression) Text() string {
	if e.e == nil {
		return ""
	}
	return e.e.text
}

// Template returns the first compiled structural template for a snippet expression.
// Use Templates when a language permits the same snippet in multiple contexts.
func (e Expression) Template() Template {
	if e.e == nil {
		return Template{}
	}
	return e.e.template
}

// Templates returns every grammar-valid interpretation of a snippet.
func (e Expression) Templates() []Template {
	if e.e == nil {
		return nil
	}
	return append([]Template(nil), e.e.templates...)
}

// Variables returns a copy of the metavariable references in source order.
func (e Expression) Variables() []VariableRef {
	if e.e == nil {
		return nil
	}
	return append([]VariableRef(nil), e.e.refs...)
}

// Children returns read-only handles for child expressions in evaluation order.
func (e Expression) Children() []Expression {
	if e.e == nil {
		return nil
	}
	out := make([]Expression, len(e.e.children))
	for i, child := range e.e.children {
		out[i] = Expression{child}
	}
	return out
}

// Constraints returns read-only handles for ordered where constraints.
func (e Expression) Constraints() []Constraint {
	if e.e == nil {
		return nil
	}
	out := make([]Constraint, len(e.e.constraints))
	for i := range e.e.constraints {
		out[i] = Constraint{c: &e.e.constraints[i]}
	}
	return out
}

// MatchRegex lets a later evaluator use the compiled RE2 expression without exposing it.
func (e Expression) MatchRegex(text string) bool {
	return e.e != nil && e.e.re != nil && e.e.re.MatchString(text)
}

// Constraint is a read-only handle to one compiled where constraint.
type Constraint struct{ c *constraint }

// LHS returns the constrained metavariable reference.
func (c Constraint) LHS() VariableRef {
	if c.c == nil {
		return VariableRef{}
	}
	return c.c.lhs
}

// RHS returns the constraint predicate expression.
func (c Constraint) RHS() Expression {
	if c.c == nil {
		return Expression{}
	}
	return Expression{c.c.rhs}
}

// Range returns the constraint's query-source range.
func (c Constraint) Range() Range {
	if c.c == nil {
		return Range{}
	}
	return c.c.rng
}

// LanguageDeclaration is the resolved language header in the program IR.
type LanguageDeclaration struct {
	Name  string
	Range Range
}

// Program is immutable and safe for concurrent use.
type Program struct {
	declaration   LanguageDeclaration
	adapter       targetLanguageAdapter
	rng           Range
	root          *expression
	variables     []Variable
	features      FeatureSet
	compileLimits CompileOptions
}

// Compatibility returns the unified compatibility identifier.
func (p *Program) Compatibility() string { return Compatibility }

// Language returns the program's resolved target language, or empty for a nil program.
func (p *Program) Language() string {
	if p == nil {
		return ""
	}
	return p.declaration.Name
}

// Declaration returns the resolved language declaration, or its zero value for a nil program.
func (p *Program) Declaration() LanguageDeclaration {
	if p == nil {
		return LanguageDeclaration{}
	}
	return p.declaration
}

// Range returns the complete query-source range, or an empty range for a nil program.
func (p *Program) Range() Range {
	if p == nil {
		return Range{}
	}
	return p.rng
}

// Root returns a read-only handle to the root expression.
func (p *Program) Root() Expression {
	if p == nil {
		return Expression{}
	}
	return Expression{p.root}
}

// Variables returns a copy of the program's variables in first-occurrence order.
func (p *Program) Variables() []Variable {
	if p == nil {
		return nil
	}
	return append([]Variable(nil), p.variables...)
}

// Features returns the set of syntax features used by the program.
func (p *Program) Features() FeatureSet {
	if p == nil {
		return 0
	}
	return p.features
}

// Compile parses, validates, resolves, and closes source before returning.
func Compile(source []byte, options CompileOptions) (*Program, error) {
	options = normalizedOptions(options)
	doc, wholeValue, compileErr := parseCompileSource(source, options)
	if compileErr != nil {
		return nil, compileErr
	}
	defer doc.close()

	c := &treeCompiler{doc: doc, options: options, names: make(map[string]VariableID)}
	whole := &wholeValue
	languageNode, patternNode, compileErr := c.compileDocumentFields(doc.root(), whole)
	if compileErr != nil {
		return nil, compileErr
	}
	if compileErr = c.compileLanguage(languageNode); compileErr != nil {
		compileErr.Range = whole
		return nil, compileErr
	}
	root, compileErr := c.compileExpression(patternNode, false)
	if compileErr != nil {
		if compileErr.Code != "LIMIT_PARSE_DEPTH" {
			compileErr.Range = whole
		}
		return nil, compileErr
	}
	if _, scopeErr := validatePossibleBindings(root, nil); scopeErr != nil {
		scopeErr.Range = whole
		return nil, scopeErr
	}
	variables := append([]Variable(nil), c.variables...)
	return &Program{declaration: LanguageDeclaration{Name: c.adapter.id, Range: c.languageRange}, adapter: c.adapter, rng: wholeValue, root: root, variables: variables, features: c.features, compileLimits: options}, nil
}

func parseCompileSource(source []byte, options CompileOptions) (*queryDocument, Range, *CompileError) {
	if len(source) > options.MaxPatternBytes {
		whole := publicRange(wholeRangeConstantMemory(source))
		return nil, Range{}, &CompileError{Code: "LIMIT_PATTERN_BYTES", Class: "resource", Message: "decoded pattern exceeds effective byte limit", Range: &whole}
	}
	whole := publicRange(rangeInBytes(source, lineStarts(source), 0, len(source)))
	doc, err := parseQueryWithMaxDepth(source, options.MaxDepth)
	if err != nil {
		if doc != nil {
			doc.close()
		}
		return nil, Range{}, parseCompileError(err, &whole)
	}
	return doc, whole, nil
}

func parseCompileError(err error, whole *Range) *CompileError {
	kind := queryMalformed
	if syntaxErr, ok := err.(*querySyntaxError); ok {
		kind = syntaxErr.Kind
	}
	if kind == queryDepthLimit {
		return &CompileError{Code: "LIMIT_PARSE_DEPTH", Class: "resource", Message: "pattern tree exceeds effective depth limit", cause: err}
	}
	code, class := "PATTERN_PARSE", "pattern"
	if kind == queryUnsupported {
		code, class = "PATTERN_UNSUPPORTED", "unsupported"
	} else if kind == queryInvalidContext {
		code = "PATTERN_INVALID_CONTEXT"
	}
	return &CompileError{Code: code, Class: class, Message: err.Error(), Range: whole, cause: err}
}

func (c *treeCompiler) compileDocumentFields(rootNode queryNode, whole *Range) (queryNode, queryNode, *CompileError) {
	if rootNode.kind() != "source_file" || rootNode.fieldName() != "" || !rootNode.named() || rootNode.extra() || rootNode.error() || rootNode.missing() || !validCompilerComments(rootNode) {
		compileErr := c.closedFailure("unexpected query root")
		compileErr.Range = whole
		return queryNode{}, queryNode{}, compileErr
	}
	var languageNode, patternNode queryNode
	for _, child := range rootNode.namedChildren() {
		if compileErr := c.acceptDocumentField(child, &languageNode, &patternNode); compileErr != nil {
			compileErr.Range = whole
			return queryNode{}, queryNode{}, compileErr
		}
	}
	if !languageNode.valid() || !patternNode.valid() {
		compileErr := c.closedFailure("incomplete query document")
		compileErr.Range = whole
		return queryNode{}, queryNode{}, compileErr
	}
	return languageNode, patternNode, nil
}

func (c *treeCompiler) acceptDocumentField(child queryNode, languageNode, patternNode *queryNode) *CompileError {
	switch {
	case child.kind() == "comment" && child.fieldName() == "":
	case child.kind() == "langdecl" && child.fieldName() == "language" && !languageNode.valid():
		*languageNode = child
	case child.fieldName() == "pattern" && !patternNode.valid():
		*patternNode = child
	default:
		return c.closedFailure("unexpected query document field")
	}
	return nil
}

func normalizedOptions(o CompileOptions) CompileOptions {
	o.MaxPatternBytes = boundedOption(o.MaxPatternBytes, defaultMaxPatternBytes, hardMaxPatternBytes)
	o.MaxRegexBytes = boundedOption(o.MaxRegexBytes, defaultMaxRegexBytes, hardMaxRegexBytes)
	o.MaxRegexInstructions = boundedOption(o.MaxRegexInstructions, defaultMaxRegexInstructions, hardMaxRegexInstructions)
	o.MaxDepth = boundedOption(o.MaxDepth, defaultMaxDepth, hardMaxDepth)
	return o
}

func boundedOption(value, fallback, maximum int) int {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func publicRange(r queryRange) Range {
	return Range{StartByte: r.StartByte, EndByte: r.EndByte, Start: Position(r.Start), End: Position(r.End)}
}

// wholeRangeConstantMemory computes an oversized-input diagnostic without the
// proportional line-start index used after the size gate. The size limit has
// priority over UTF-8 validity; malformed bytes each count as one scalar column.
func wholeRangeConstantMemory(source []byte) queryRange {
	line, column := 1, 1
	for pos := 0; pos < len(source); {
		if source[pos] == '\n' {
			line, column, pos = line+1, 1, pos+1
			continue
		}
		_, size := utf8.DecodeRune(source[pos:])
		if size == 0 {
			size = 1
		}
		pos += size
		column++
	}
	return queryRange{StartByte: 0, EndByte: len(source), Start: queryPosition{Line: 1, Column: 1}, End: queryPosition{Line: line, Column: column}}
}

type treeCompiler struct {
	doc           *queryDocument
	options       CompileOptions
	names         map[string]VariableID
	variables     []Variable
	features      FeatureSet
	adapter       targetLanguageAdapter
	languageRange Range
}

func (c *treeCompiler) compileLanguage(node queryNode) *CompileError {
	children := nonCommentNamedChildren(node)
	if !node.named() || node.extra() || !validCompilerComments(node) || len(children) != 1 || !isNode(children[0], "languageName", "name", true) {
		return c.closedFailure("unexpected language declaration")
	}
	adapter, ok := targetLanguageByID(children[0].text())
	if !ok {
		return c.closedFailure("unsupported target language " + children[0].text())
	}
	concrete := nonCommentChildren(node)
	if len(concrete) != 2 || !isCompilerLeaf(concrete[0], "language", "", false) || !isNode(concrete[1], "languageName", "name", true) {
		return c.closedFailure("unexpected language declaration shape")
	}
	nameChildren := nonCommentChildren(concrete[1])
	if !validCompilerComments(concrete[1]) || len(nameChildren) != 1 || !isCompilerLeaf(nameChildren[0], adapter.id, "", false) {
		return c.closedFailure("unexpected language name shape")
	}
	c.adapter = adapter
	c.languageRange = publicRange(node.byteRange())
	return nil
}

func (c *treeCompiler) compileExpression(node queryNode, constraintPredicateAllowed bool) (*expression, *CompileError) {
	if !node.valid() || !node.named() || node.error() || node.missing() {
		return nil, c.closedFailure("invalid query node")
	}
	switch node.kind() {
	case "codeSnippet":
		return c.compileSnippet(node)
	case "regexPattern":
		if !constraintPredicateAllowed {
			return nil, c.failure("PATTERN_INVALID_CONTEXT", "pattern", "regex is valid only as a direct constraint predicate", nil)
		}
		return c.compileRegex(node)
	case "emptyPredicate":
		if !constraintPredicateAllowed {
			return nil, c.failure("PATTERN_INVALID_CONTEXT", "pattern", "empty is valid only as a direct constraint predicate", nil)
		}
		c.features |= FeatureEmpty
		return &expression{kind: KindEmpty, rng: publicRange(node.byteRange())}, nil
	case "nodeLike":
		return c.compileNodeLike(node)
	case "patternAs":
		return c.compileAs(node)
	case "patternParent":
		concrete := nonCommentChildren(node)
		if len(concrete) != 5 || !validCompilerComments(node) || !isCompilerLeaf(concrete[0], "parent", "", false) || !isCompilerLeaf(concrete[1], "kind", "", false) || !isCompilerLeaf(concrete[2], "(", "", false) || !isCompilerLeaf(concrete[3], "\"function\"", "", false) || !isCompilerLeaf(concrete[4], ")", "", false) {
			return nil, c.closedFailure("unexpected parent category")
		}
		c.features |= FeatureParent
		return &expression{kind: KindParent, rng: publicRange(node.byteRange()), text: "function"}, nil
	case "patternAnd":
		return c.compileBlock(node, KindAnd, FeatureAnd, "and")
	case "patternOr":
		return c.compileBlock(node, KindOr, FeatureOr, "or")
	case "patternNot":
		return c.compileUnary(node, KindNot, FeatureNot, "not")
	case "patternMaybe":
		return c.compileUnary(node, KindMaybe, FeatureMaybe, "maybe")
	case "patternContains":
		return c.compileUnary(node, KindContains, FeatureContains, "contains")
	case "within":
		return c.compileUnary(node, KindWithin, FeatureWithin, "within")
	case "patternWhere":
		return c.compileWhere(node)
	default:
		return nil, c.closedFailure("unexpected named query node " + node.kind())
	}
}

// compileAs binds the exact named root matched by a node-like selector.
func (c *treeCompiler) compileAs(node queryNode) (*expression, *CompileError) {
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(concrete) != 3 || !isNode(concrete[0], "nodeLike", "pattern", true) ||
		!isCompilerLeaf(concrete[1], "as", "", false) || !isCompilerLeaf(concrete[2], "variable", "variable", true) {
		return nil, c.closedFailure("invalid node capture")
	}
	child, err := c.compileExpression(concrete[0], false)
	if err != nil {
		return nil, err
	}
	position := concrete[2].byteRange()
	ref := c.variable(concrete[2].text(), position.StartByte, position.EndByte)
	if ref.Anonymous {
		return nil, c.failure("PATTERN_INVALID_CONTEXT", "pattern", "capture requires a named variable", nil)
	}
	c.features |= FeatureAs
	return &expression{kind: KindAs, rng: publicRange(node.byteRange()), refs: []VariableRef{ref}, children: []*expression{child}}, nil
}

// compileNodeLike captures direct grammar fields or positional named children.
func (c *treeCompiler) compileNodeLike(node queryNode) (*expression, *CompileError) {
	concrete := nonCommentChildren(node)
	if len(concrete) < 3 || !isNode(concrete[0], "name", "name", true) || !isCompilerLeaf(concrete[1], "(", "", false) || !isCompilerLeaf(concrete[len(concrete)-1], ")", "", false) {
		return nil, c.closedFailure("invalid node pattern")
	}
	if !validNodeSelectorName(concrete[0].text()) {
		return nil, c.closedFailure("invalid syntax-node kind")
	}
	if !parser.NewParser().GetGrammar(c.adapter.id).NodeKind(concrete[0].text()) {
		return nil, c.failure("PATTERN_INVALID_SNIPPET", "pattern", "unknown syntax-node kind "+concrete[0].text(), nil)
	}
	result := &expression{kind: KindNodeLike, rng: publicRange(node.byteRange()), text: concrete[0].text()}
	for _, item := range concrete[2 : len(concrete)-1] {
		if isCompilerLeaf(item, ",", "named_args", false) {
			continue
		}
		if item.kind() != "namedArg" || item.fieldName() != "named_args" {
			return nil, c.closedFailure("invalid node field pattern")
		}
		parts := nonCommentChildren(item)
		field := ""
		var pattern queryNode
		switch {
		case len(parts) == 1 && parts[0].fieldName() == "variable" && (parts[0].kind() == "variable" || parts[0].kind() == "nodeLike"):
			pattern = parts[0]
		case len(parts) == 3 && isNode(parts[0], "name", "name", true) && isCompilerLeaf(parts[1], "=", "", false) && parts[2].fieldName() == "pattern":
			field = parts[0].text()
			if !validNodeSelectorName(field) {
				return nil, c.closedFailure("invalid syntax-node field")
			}
			if parser.NewParser().GetGrammar(c.adapter.id).FieldCardinality(concrete[0].text(), field) == parser.GrammarCardinalityUnknown {
				return nil, c.failure("PATTERN_INVALID_SNIPPET", "pattern", "unknown syntax-node field "+field+" on "+result.text, nil)
			}
			pattern = parts[2]
		default:
			return nil, c.closedFailure("invalid node field pattern")
		}
		var child *expression
		if pattern.kind() == "variable" {
			rng := pattern.byteRange()
			ref := c.variable(pattern.text(), rng.StartByte, rng.EndByte)
			child = &expression{kind: KindNodeCapture, rng: publicRange(rng), refs: []VariableRef{ref}}
			result.refs = append(result.refs, ref)
		} else if pattern.kind() == "nodeLike" {
			var err *CompileError
			child, err = c.compileNodeLike(pattern)
			if err != nil {
				return nil, err
			}
			result.refs = append(result.refs, child.refs...)
		} else {
			return nil, c.failure("PATTERN_INVALID_CONTEXT", "pattern", "node fields support only nested nodes or metavariables", nil)
		}
		result.children = append(result.children, child)
		result.fields = append(result.fields, field)
	}
	c.features |= FeatureNodeLike
	if len(result.refs) > 0 {
		c.features |= FeatureVariables
	}
	return result, nil
}

func validNodeSelectorName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for index := 0; index < len(name); index++ {
		char := name[index]
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func (c *treeCompiler) compileSnippet(node queryNode) (*expression, *CompileError) {
	leaf, compileErr := c.snippetLeaf(node)
	if compileErr != nil {
		return nil, compileErr
	}
	decoded, refs, placeholders, compileErr := c.decodeSnippet(leaf)
	if compileErr != nil {
		return nil, compileErr
	}
	templates, code, templateErr := c.adapter.compileTemplates(decodedSnippet{text: string(decoded), placeholders: placeholders}, c.options.MaxDepth)
	if templateErr != nil {
		class := "pattern"
		if code == "LIMIT_PARSE_DEPTH" {
			class = "resource"
		}
		return nil, c.failure(code, class, templateErr.Error(), templateErr)
	}
	c.features |= FeatureSnippet
	return &expression{kind: KindSnippet, rng: publicRange(node.byteRange()), text: string(decoded), refs: refs, template: templates[0], templates: templates}, nil
}

func (c *treeCompiler) snippetLeaf(node queryNode) (queryNode, *CompileError) {
	children := nonCommentNamedChildren(node)
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(children) != 1 || len(concrete) != 1 || !isCompilerLeaf(children[0], "backtickSnippet", "source", true) {
		return queryNode{}, c.closedFailure("unexpected snippet field")
	}
	payload := children[0].text()
	if len(payload) < 2 || payload[0] != '`' || payload[len(payload)-1] != '`' {
		return queryNode{}, c.closedFailure("unexpected snippet payload")
	}
	return children[0], nil
}

func (c *treeCompiler) decodeSnippet(leaf queryNode) ([]byte, []VariableRef, []snippetPlaceholder, *CompileError) {
	payload := []byte(leaf.text())
	base := leaf.byteRange().StartByte
	decoded := make([]byte, 0, len(payload)-2)
	var refs []VariableRef
	var placeholders []snippetPlaceholder
	for pos := 1; pos < len(payload)-1; {
		if payload[pos] == '\\' {
			if pos+1 >= len(payload)-1 || payload[pos+1] != '`' && payload[pos+1] != '\\' {
				return nil, nil, nil, c.closedFailure("invalid validated snippet escape")
			}
			decoded = append(decoded, payload[pos+1])
			pos += 2
			continue
		}
		if payload[pos] != '$' {
			decoded = append(decoded, payload[pos])
			pos++
			continue
		}
		end, compileErr := c.snippetMetavariableEnd(payload, pos)
		if compileErr != nil {
			return nil, nil, nil, compileErr
		}
		ref := c.variable(string(payload[pos:end]), base+pos, base+end)
		refs = append(refs, ref)
		decodedStart := len(decoded)
		decoded = append(decoded, payload[pos:end]...)
		placeholders = append(placeholders, snippetPlaceholder{start: decodedStart, end: len(decoded), ref: ref})
		pos = end
	}
	return decoded, refs, placeholders, nil
}

func (c *treeCompiler) snippetMetavariableEnd(payload []byte, pos int) (int, *CompileError) {
	end := pos + 1
	if end < len(payload)-1 && payload[end] == '_' {
		return end + 1, nil
	}
	if end >= len(payload)-1 || !isASCIIAlpha(payload[end]) {
		return 0, c.closedFailure("invalid validated metavariable")
	}
	end++
	for end < len(payload)-1 && isMetaContinue(payload[end]) {
		end++
	}
	return end, nil
}

func (c *treeCompiler) compileRegex(node queryNode) (*expression, *CompileError) {
	payload, compileErr := c.regexPayload(node)
	if compileErr != nil {
		return nil, compileErr
	}
	decoded, compileErr := c.decodeRegex(payload)
	if compileErr != nil {
		return nil, compileErr
	}
	re, compileErr := c.compileDecodedRegex(decoded)
	if compileErr != nil {
		return nil, compileErr
	}
	c.features |= FeatureRegex
	return &expression{kind: KindRegex, rng: publicRange(node.byteRange()), text: string(decoded), re: re}, nil
}

func (c *treeCompiler) regexPayload(node queryNode) ([]byte, *CompileError) {
	children := nonCommentNamedChildren(node)
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(children) != 1 || len(concrete) != 1 || !isCompilerLeaf(children[0], "regex", "regex", true) {
		return nil, c.closedFailure("unexpected regex field")
	}
	payload := []byte(children[0].text())
	if len(payload) < 3 || payload[0] != 'r' || payload[1] != '"' || payload[len(payload)-1] != '"' {
		return nil, c.closedFailure("unexpected regex payload")
	}
	return payload, nil
}

func (c *treeCompiler) decodeRegex(payload []byte) ([]byte, *CompileError) {
	decoded := make([]byte, 0, len(payload)-3)
	for pos := 2; pos < len(payload)-1; {
		if payload[pos] != '\\' {
			decoded = append(decoded, payload[pos])
			pos++
			continue
		}
		escapeStart := pos
		escaped, next, compileErr := c.decodeRegexEscape(payload, pos+1, escapeStart)
		if compileErr != nil {
			return nil, compileErr
		}
		decoded = append(decoded, escaped...)
		pos = next
	}
	return decoded, nil
}

func (c *treeCompiler) decodeRegexEscape(payload []byte, pos, escapeStart int) ([]byte, int, *CompileError) {
	switch payload[pos] {
	case '"', '\\':
		return payload[pos : pos+1], pos + 1, nil
	case 'n':
		return []byte{'\n'}, pos + 1, nil
	case 'r':
		return []byte{'\r'}, pos + 1, nil
	case 't':
		return []byte{'\t'}, pos + 1, nil
	case 'x', 'u':
		return c.decodeRegexHexEscape(payload, pos, escapeStart)
	default:
		return nil, 0, c.closedFailure(fmt.Sprintf("invalid validated regex escape at %d", escapeStart))
	}
}

func (c *treeCompiler) decodeRegexHexEscape(payload []byte, pos, escapeStart int) ([]byte, int, *CompileError) {
	digits := 2
	if payload[pos] == 'u' {
		digits = 4
	}
	pos++
	value, parseErr := strconv.ParseUint(string(payload[pos:pos+digits]), 16, 16)
	if parseErr != nil {
		return nil, 0, c.closedFailure(fmt.Sprintf("invalid validated regex escape at %d", escapeStart))
	}
	pos += digits
	if value >= 0xD800 && value <= 0xDFFF {
		return nil, 0, c.failure("PATTERN_INVALID_REGEX", "pattern", "regex escape decodes to invalid Unicode scalar", nil)
	}
	return []byte(string(rune(value))), pos, nil
}

func (c *treeCompiler) compileDecodedRegex(decoded []byte) (*regexp.Regexp, *CompileError) {
	if len(decoded) > c.options.MaxRegexBytes {
		return nil, c.failure("PATTERN_INVALID_REGEX", "pattern", "decoded regex exceeds effective byte limit", nil)
	}
	ast, parseErr := syntax.Parse(string(decoded), syntax.Perl)
	if parseErr != nil {
		return nil, c.failure("PATTERN_INVALID_REGEX", "pattern", "invalid RE2 expression", parseErr)
	}
	prog, compileErr := syntax.Compile(ast.Simplify())
	if compileErr != nil {
		return nil, c.failure("PATTERN_INVALID_REGEX", "pattern", "invalid RE2 expression", compileErr)
	}
	if len(prog.Inst) > c.options.MaxRegexInstructions {
		return nil, c.failure("PATTERN_INVALID_REGEX", "pattern", "compiled regex exceeds effective instruction limit", nil)
	}
	re, regexpErr := regexp.Compile(string(decoded))
	if regexpErr != nil {
		return nil, c.failure("PATTERN_INVALID_REGEX", "pattern", "invalid RE2 expression", regexpErr)
	}
	return re, nil
}

func (c *treeCompiler) compileBlock(node queryNode, kind Kind, feature FeatureSet, keyword string) (*expression, *CompileError) {
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(concrete) < 6 || !isCompilerLeaf(concrete[0], keyword, "", false) || !isCompilerLeaf(concrete[1], "{", "", false) || !isCompilerLeaf(concrete[len(concrete)-1], "}", "", false) {
		return nil, c.closedFailure("unexpected " + keyword + " shape")
	}
	children, expectPattern, compileErr := c.compileBlockChildren(concrete[2:len(concrete)-1], keyword)
	if compileErr != nil {
		return nil, compileErr
	}
	if len(children) < 2 || !expectPattern && concrete[len(concrete)-2].kind() == "," {
		return nil, c.closedFailure("unexpected " + keyword + " arity")
	}
	c.features |= feature
	return &expression{kind: kind, rng: publicRange(node.byteRange()), children: children}, nil
}

func (c *treeCompiler) compileBlockChildren(nodes []queryNode, keyword string) ([]*expression, bool, *CompileError) {
	var children []*expression
	expectPattern := true
	for _, child := range nodes {
		if !expectPattern {
			if !isCompilerLeaf(child, ",", "patterns", false) {
				return nil, false, c.closedFailure("unexpected " + keyword + " delimiter")
			}
			expectPattern = true
			continue
		}
		if !child.named() || child.fieldName() != "patterns" {
			return nil, false, c.closedFailure("unexpected " + keyword + " child")
		}
		compiled, compileErr := c.compileExpression(child, false)
		if compileErr != nil {
			return nil, false, compileErr
		}
		children = append(children, compiled)
		expectPattern = false
	}
	return children, expectPattern, nil
}

func (c *treeCompiler) compileUnary(node queryNode, kind Kind, feature FeatureSet, keyword string) (*expression, *CompileError) {
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(concrete) != 2 || !isCompilerLeaf(concrete[0], keyword, "", false) || !concrete[1].named() || concrete[1].extra() || concrete[1].error() || concrete[1].missing() {
		return nil, c.closedFailure("unexpected " + keyword + " shape")
	}
	wantField := "pattern"
	if keyword == "contains" {
		wantField = "contains"
	}
	if concrete[1].fieldName() != wantField {
		return nil, c.closedFailure("unexpected " + keyword + " field")
	}
	child, compileErr := c.compileExpression(concrete[1], false)
	if compileErr != nil {
		return nil, compileErr
	}
	c.features |= feature
	return &expression{kind: kind, rng: publicRange(node.byteRange()), children: []*expression{child}}, nil
}

func (c *treeCompiler) compileWhere(node queryNode) (*expression, *CompileError) {
	children, compileErr := c.whereChildren(node)
	if compileErr != nil {
		return nil, compileErr
	}
	prefix, compileErr := c.compileExpression(children[0], false)
	if compileErr != nil {
		return nil, compileErr
	}
	if prefix.kind == KindRegex {
		return nil, c.failure("PATTERN_INVALID_CONTEXT", "pattern", "regex is valid only as a direct constraint predicate", nil)
	}
	constraints, compileErr := c.compileConstraintBlock(children[1])
	if compileErr != nil {
		return nil, compileErr
	}
	c.features |= FeatureWhere
	return &expression{kind: KindWhere, rng: publicRange(node.byteRange()), children: []*expression{prefix}, constraints: constraints}, nil
}

func (c *treeCompiler) whereChildren(node queryNode) ([]queryNode, *CompileError) {
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(concrete) != 3 || !concrete[0].named() || concrete[0].fieldName() != "pattern" || !isCompilerLeaf(concrete[1], "where", "", false) || !isNode(concrete[2], "predicateAnd", "side_condition", true) {
		return nil, c.closedFailure("unexpected where shape")
	}
	children := nonCommentNamedChildren(node)
	if len(children) != 2 || children[0].fieldName() != "pattern" || children[1].kind() != "predicateAnd" || children[1].fieldName() != "side_condition" {
		return nil, c.closedFailure("unexpected where fields")
	}
	return children, nil
}

func (c *treeCompiler) compileConstraintBlock(node queryNode) ([]constraint, *CompileError) {
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(concrete) < 3 || !isCompilerLeaf(concrete[0], "{", "", false) || !isCompilerLeaf(concrete[len(concrete)-1], "}", "", false) {
		return nil, c.closedFailure("unexpected constraint block")
	}
	constraints, compileErr := c.compileConstraintItems(concrete[1 : len(concrete)-1])
	if compileErr != nil {
		return nil, compileErr
	}
	if len(constraints) == 0 {
		return nil, c.closedFailure("empty constraint block")
	}
	return constraints, nil
}

func (c *treeCompiler) compileConstraintItems(nodes []queryNode) ([]constraint, *CompileError) {
	var constraints []constraint
	expectPredicate := true
	for _, predicate := range nodes {
		if !expectPredicate {
			if !isCompilerLeaf(predicate, ",", "predicates", false) {
				return nil, c.closedFailure("unexpected constraint delimiter")
			}
			expectPredicate = true
			continue
		}
		if !predicate.named() || predicate.kind() != "predicateMatch" || predicate.fieldName() != "predicates" {
			return nil, c.closedFailure("unexpected constraint predicate")
		}
		compiled, compileErr := c.compileConstraint(predicate)
		if compileErr != nil {
			return nil, compileErr
		}
		constraints = append(constraints, compiled)
		expectPredicate = false
	}
	return constraints, nil
}

func (c *treeCompiler) compileConstraint(node queryNode) (constraint, *CompileError) {
	concrete := nonCommentChildren(node)
	if !validCompilerComments(node) || len(concrete) != 3 || !isCompilerLeaf(concrete[0], "variable", "left", true) || !isCompilerLeaf(concrete[1], "<:", "", false) || !concrete[2].named() || concrete[2].extra() || concrete[2].error() || concrete[2].missing() || concrete[2].fieldName() != "right" {
		return constraint{}, c.closedFailure("unexpected constraint shape")
	}
	variableRange := concrete[0].byteRange()
	lhs := c.variable(concrete[0].text(), variableRange.StartByte, variableRange.EndByte)
	if lhs.Anonymous {
		return constraint{}, c.failure("PATTERN_INVALID_CONTEXT", "pattern", "anonymous wildcard is invalid on a constraint left side", nil)
	}
	rhs, compileErr := c.compileExpression(concrete[2], true)
	if compileErr != nil {
		return constraint{}, compileErr
	}
	return constraint{lhs: lhs, rhs: rhs, rng: publicRange(node.byteRange())}, nil
}

func (c *treeCompiler) variable(name string, start, end int) VariableRef {
	rng := publicRange(c.doc.rangeForBytes(start, end))
	c.features |= FeatureVariables
	if name == "$_" {
		return VariableRef{Name: name, Anonymous: true, Range: rng}
	}
	id, ok := c.names[name]
	if !ok {
		id = VariableID(len(c.variables) + 1)
		c.names[name] = id
		c.variables = append(c.variables, Variable{ID: id, Name: name, Range: rng})
	}
	return VariableRef{ID: id, Name: name, Range: rng}
}

func isCompilerLeaf(node queryNode, kind, field string, named bool) bool {
	return isNode(node, kind, field, named) && !node.extra() && len(node.children()) == 0
}

func validCompilerComments(node queryNode) bool {
	for _, child := range node.children() {
		if child.kind() == "comment" && (!child.named() || !child.extra() || child.fieldName() != "" || len(child.children()) != 0) {
			return false
		}
	}
	return true
}

func nonCommentChildren(node queryNode) []queryNode {
	children := node.children()
	out := make([]queryNode, 0, len(children))
	for _, child := range children {
		if child.kind() != "comment" {
			out = append(out, child)
		}
	}
	return out
}

func nonCommentNamedChildren(node queryNode) []queryNode {
	children := node.namedChildren()
	out := make([]queryNode, 0, len(children))
	for _, child := range children {
		if child.kind() != "comment" {
			out = append(out, child)
		}
	}
	return out
}

func (c *treeCompiler) closedFailure(message string) *CompileError {
	return c.failure("PATTERN_PARSE", "pattern", message, nil)
}

func (c *treeCompiler) failure(code, class, message string, cause error) *CompileError {
	return &CompileError{Code: code, Class: class, Message: message, cause: cause}
}

type variableScope map[VariableID]bool

func validatePossibleBindings(e *expression, incoming variableScope) (variableScope, *CompileError) {
	if e == nil {
		return nil, bindingValidationError("invalid compiled expression")
	}
	switch e.kind {
	case KindSnippet, KindNodeLike:
		return validateSnippetBindings(e, incoming), nil
	case KindAs:
		available, err := validateUnaryBindings(e, incoming)
		if err != nil {
			return nil, err
		}
		return validateSnippetBindings(e, available), nil
	case KindRegex, KindEmpty, KindParent:
		return cloneVariableScope(incoming), nil
	case KindAnd, KindOr:
		return validateBranchBindings(e, incoming)
	case KindNot:
		return validateNotBindings(e, incoming)
	case KindMaybe, KindContains, KindWithin:
		return validateUnaryBindings(e, incoming)
	case KindWhere:
		return validateWhereBindings(e, incoming)
	default:
		return nil, bindingValidationError("unknown compiled expression")
	}
}

func validateSnippetBindings(e *expression, incoming variableScope) variableScope {
	out := cloneVariableScope(incoming)
	for _, ref := range e.refs {
		if !ref.Anonymous {
			out[ref.ID] = true
		}
	}
	return out
}

func validateBranchBindings(e *expression, incoming variableScope) (variableScope, *CompileError) {
	out := cloneVariableScope(incoming)
	for _, child := range e.children {
		childOut, err := validatePossibleBindings(child, incoming)
		if err != nil {
			return nil, err
		}
		unionVariableScope(out, childOut)
	}
	return out, nil
}

func validateNotBindings(e *expression, incoming variableScope) (variableScope, *CompileError) {
	if len(e.children) != 1 {
		return nil, bindingValidationError("invalid not expression")
	}
	if _, err := validatePossibleBindings(e.children[0], incoming); err != nil {
		return nil, err
	}
	return cloneVariableScope(incoming), nil
}

func validateUnaryBindings(e *expression, incoming variableScope) (variableScope, *CompileError) {
	if len(e.children) != 1 {
		return nil, bindingValidationError("invalid unary expression")
	}
	return validatePossibleBindings(e.children[0], incoming)
}

func validateWhereBindings(e *expression, incoming variableScope) (variableScope, *CompileError) {
	if len(e.children) != 1 {
		return nil, bindingValidationError("invalid where expression")
	}
	available, err := validatePossibleBindings(e.children[0], incoming)
	if err != nil {
		return nil, err
	}
	for _, item := range e.constraints {
		if !available[item.lhs.ID] {
			message := fmt.Sprintf("constraint left side %s is not defined", item.lhs.Name)
			return nil, &CompileError{Code: "PATTERN_INVALID_CONTEXT", Class: "pattern", Message: message}
		}
		available, err = validatePossibleBindings(item.rhs, available)
		if err != nil {
			return nil, err
		}
	}
	return available, nil
}

func bindingValidationError(message string) *CompileError {
	return &CompileError{Code: "PATTERN_PARSE", Class: "pattern", Message: message}
}

func cloneVariableScope(scope variableScope) variableScope {
	out := make(variableScope, len(scope))
	for id := range scope {
		out[id] = true
	}
	return out
}

func unionVariableScope(dst, src variableScope) {
	for id := range src {
		dst[id] = true
	}
}
