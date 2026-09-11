package gritql

import (
	"reflect"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// MatchTarget is either one concrete syntax node or a consecutive repeated-child
// span of one parent. Its zero value is invalid.
type MatchTarget struct {
	node       parser.Node
	parent     parser.Node
	viewNode   parser.ViewNode
	viewParent parser.ViewNode
	kind       string
	field      string
	childStart int
	childEnd   int
	sequence   bool
}

// NodeTarget constructs a target for an ordinary template (including a complete
// source_file template).
func NodeTarget(node parser.Node) MatchTarget { return MatchTarget{node: node} }

// SequenceTarget constructs an authored statement/declaration sequence from the
// half-open direct-child span [childStart, childEnd) of parent. Invalid indexes
// and parent/kind combinations are rejected during matching.
func SequenceTarget(kind string, parent parser.Node, childStart, childEnd int) MatchTarget {
	return MatchTarget{kind: kind, parent: parent, childStart: childStart, childEnd: childEnd, sequence: true}
}

// RepeatedSequenceTarget constructs a grammar-generic list target. The parent
// kind and field name must identify a repeated position in the pinned Go grammar;
// the span may contain only that position's named elements and intervening syntax.
func RepeatedSequenceTarget(parent parser.Node, field string, childStart, childEnd int) MatchTarget {
	return MatchTarget{kind: "list_sequence", field: field, parent: parent, childStart: childStart, childEnd: childEnd, sequence: true}
}

func repeatedSequenceTarget(parent parser.Node, field string, childStart, childEnd int) MatchTarget {
	return RepeatedSequenceTarget(parent, field, childStart, childEnd)
}

func viewNodeTarget(node parser.ViewNode) MatchTarget {
	return MatchTarget{viewNode: node}
}

func viewSequenceTarget(kind string, parent parser.ViewNode, childStart, childEnd int) MatchTarget {
	return MatchTarget{kind: kind, viewParent: parent, childStart: childStart, childEnd: childEnd, sequence: true}
}

func viewRepeatedSequenceTarget(parent parser.ViewNode, field string, childStart, childEnd int) MatchTarget {
	return MatchTarget{kind: "list_sequence", field: field, viewParent: parent, childStart: childStart, childEnd: childEnd, sequence: true}
}

type frozenMatchTarget struct {
	node     parser.SyntaxNode
	kind     string
	field    string
	children []parser.SyntaxNode
	anchor   parser.Range
	sequence bool
}

// BindingKind distinguishes one structural node from a repeated-child list.
type BindingKind uint8

// BindingNode and the following constants identify supported binding shapes.
const (
	BindingNode BindingKind = iota + 1
	BindingList
)

func (k BindingKind) String() string {
	switch k {
	case BindingNode:
		return "node"
	case BindingList:
		return "list"
	default:
		return "unknown"
	}
}

// StructuralNode is an immutable normalized concrete syntax tree.
type StructuralNode struct{ node *normalizedNode }

type normalizedNode struct {
	kind     string
	lexeme   string
	named    bool
	children []normalizedNode
}

func (n normalizedNode) MarshalJSON() ([]byte, error) { return marshalNormalizedNode(n) }

// Valid reports whether the handle refers to a normalized structural node.
func (n StructuralNode) Valid() bool { return n.node != nil }

// Kind returns the grammar node or token kind, or empty for an invalid handle.
func (n StructuralNode) Kind() string {
	if n.node == nil {
		return ""
	}
	return n.node.kind
}

// NodeKind is non-empty for normalized syntax nodes.
func (n StructuralNode) NodeKind() string {
	if n.node == nil || !n.node.named {
		return ""
	}
	return n.node.kind
}

// TokenKind is non-empty for normalized leaf tokens.
func (n StructuralNode) TokenKind() string {
	if n.node == nil || n.node.named {
		return ""
	}
	return n.node.kind
}

// IsToken reports whether the normalized value represents an unnamed leaf token.
func (n StructuralNode) IsToken() bool { return n.node != nil && !n.node.named }

// Lexeme returns the normalized leaf's exact significant source text.
func (n StructuralNode) Lexeme() string {
	if n.node == nil {
		return ""
	}
	return n.node.lexeme
}

// Children returns immutable handles to normalized children in structural order.
func (n StructuralNode) Children() []StructuralNode {
	if n.node == nil {
		return nil
	}
	if n.node.named && len(n.node.children) == 0 {
		token := normalizedNode{kind: normalizedLeafTokenKind(n.node.kind), lexeme: n.node.lexeme}
		return []StructuralNode{{node: &token}}
	}
	out := make([]StructuralNode, len(n.node.children))
	for i := range n.node.children {
		out[i] = StructuralNode{node: &n.node.children[i]}
	}
	return out
}

// MarshalJSON returns the normalized structural tree representation, or JSON null for an invalid handle.
func (n StructuralNode) MarshalJSON() ([]byte, error) {
	if n.node == nil {
		return []byte("null"), nil
	}
	return marshalNormalizedNode(*n.node)
}

type bindingValue struct {
	variable     VariableRef
	kind         BindingKind
	rng          parser.Range
	ranges       []parser.Range
	structural   []normalizedNode
	equality     []normalizedNode
	sourceRanges []parser.Range
	sourceKinds  []string
	text         string
}

// BoundValue is an immutable metavariable binding.
type BoundValue struct{ value *bindingValue }

// Valid reports whether the handle refers to a committed binding value.
func (b BoundValue) Valid() bool { return b.value != nil }

// Variable returns the compiled metavariable reference associated with the binding.
func (b BoundValue) Variable() VariableRef {
	if b.value == nil {
		return VariableRef{}
	}
	return b.value.variable
}

// Kind returns whether the value binds one node or a node list.
func (b BoundValue) Kind() BindingKind {
	if b.value == nil {
		return 0
	}
	return b.value.kind
}

// Range returns the complete source span of the bound value.
func (b BoundValue) Range() parser.Range {
	if b.value == nil {
		return parser.Range{}
	}
	return b.value.rng
}

// Ranges returns a copy of the element-aligned source ranges.
func (b BoundValue) Ranges() []parser.Range {
	if b.value == nil {
		return nil
	}
	return append([]parser.Range(nil), b.value.ranges...)
}

// Structural returns immutable normalized values aligned with Ranges.
func (b BoundValue) Structural() []StructuralNode {
	if b.value == nil {
		return nil
	}
	out := make([]StructuralNode, len(b.value.structural))
	for i := range b.value.structural {
		out[i] = StructuralNode{node: &b.value.structural[i]}
	}
	return out
}

// Text returns the exact source text spanning the binding's elements.
func (b BoundValue) Text() string {
	if b.value == nil {
		return ""
	}
	return b.value.text
}

// BindingSet is an immutable set keyed by VariableID. The zero value is empty.
type BindingSet struct{ values map[VariableID]*bindingValue }

// Lookup returns the binding for id when one is present.
func (s BindingSet) Lookup(id VariableID) (BoundValue, bool) {
	v, ok := s.values[id]
	return BoundValue{value: v}, ok
}

// Len returns the number of committed named metavariable bindings.
func (s BindingSet) Len() int { return len(s.values) }

// All returns bindings ordered by ascending variable identifier.
func (s BindingSet) All() []BoundValue {
	ids := make([]int, 0, len(s.values))
	for id := range s.values {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	out := make([]BoundValue, len(ids))
	for i, id := range ids {
		out[i] = BoundValue{value: s.values[VariableID(id)]}
	}
	return out
}

// MatchResult contains the exact significant target range and committed bindings.
type MatchResult struct {
	rng      parser.Range
	bindings BindingSet
}

// Range returns the exact significant source range matched by the template.
func (m MatchResult) Range() parser.Range { return m.rng }

// Bindings returns the immutable binding set committed by the match.
func (m MatchResult) Bindings() BindingSet { return m.bindings }

// MatchTemplate matches a compiled template against one target with no incoming
// bindings. Every invocation is independent and safe for concurrent use.
func MatchTemplate(t Template, target MatchTarget) (MatchResult, bool) {
	return MatchTemplateWithBindings(t, target, BindingSet{})
}

// MatchNode is a convenience wrapper for ordinary node targets.
func MatchNode(t Template, node parser.Node) (MatchResult, bool) {
	return MatchTemplate(t, NodeTarget(node))
}

// MatchTemplateWithBindings matches using an immutable incoming binding set.
// Failed alternatives and a failed complete attempt never modify the input.
func MatchTemplateWithBindings(t Template, target MatchTarget, incoming BindingSet) (MatchResult, bool) {
	return matchTemplateWithBudget(t, target, incoming, nil)
}

func matchTemplateWithBudget(t Template, target MatchTarget, incoming BindingSet, budget *evaluationBudget) (MatchResult, bool) {
	if !takeMatchStep(budget) || !t.Valid() {
		return MatchResult{}, false
	}
	frozen, ok := freezeMatchTarget(target)
	if !ok {
		return MatchResult{}, false
	}
	state, rng, ok := matchFrozenTemplate(t, frozen, cloneBindingMap(incoming.values), budget)
	if !ok || budget != nil && budget.err != nil {
		return MatchResult{}, false
	}
	return MatchResult{rng: rng, bindings: BindingSet{values: state}}, true
}

func matchFrozenTemplate(t Template, target frozenMatchTarget, state map[VariableID]*bindingValue, budget *evaluationBudget) (map[VariableID]*bindingValue, parser.Range, bool) {
	if t.rootSlot != nil {
		return matchFrozenRootSlot(t, target, state, budget)
	}
	if target.sequence {
		return matchFrozenSequence(t, target, state, budget)
	}
	if !target.node.Valid() || target.node.HasError() {
		return nil, parser.Range{}, false
	}
	return matchNode(t.root, target.node, state, budget)
}

func matchFrozenRootSlot(t Template, target frozenMatchTarget, state map[VariableID]*bindingValue, budget *evaluationBudget) (map[VariableID]*bindingValue, parser.Range, bool) {
	if !rootSlotTargetAccepted(t, target) {
		return nil, parser.Range{}, false
	}
	return matchRootSlot(t.rootSlot, target, state, budget)
}

func rootSlotTargetAccepted(t Template, target frozenMatchTarget) bool {
	if t.rootSlot.cardinality != SlotMany {
		return !target.sequence && rootCategoryAccepts(t.context, target.node.Kind())
	}
	if !target.sequence || target.kind != "list_sequence" && target.kind != sequenceKindForContext(t.context) {
		return false
	}
	return sequenceCategoryAccepts(t.context, listElements(target.children, target.field))
}

func matchFrozenSequence(t Template, target frozenMatchTarget, state map[VariableID]*bindingValue, budget *evaluationBudget) (map[VariableID]*bindingValue, parser.Range, bool) {
	if target.kind != t.root.kind {
		return nil, parser.Range{}, false
	}
	children := significantNodes(target.children)
	children = canonicalSequenceChildren(t.root.children, children)
	next, ok := matchChildren(t.root.children, children, 0, 0, state, target.anchor, budget)
	if !ok {
		return nil, parser.Range{}, false
	}
	return next, nodesRange(children, target.anchor), true
}

func takeMatchStep(budget *evaluationBudget) bool {
	return budget == nil || budget.take()
}

func freezeMatchTarget(target MatchTarget) (frozenMatchTarget, bool) {
	if !target.sequence {
		if target.viewNode.Valid() {
			node, ok := target.viewNode.Snapshot()
			return frozenMatchTarget{node: node}, ok
		}
		node, ok := target.node.Snapshot()
		return frozenMatchTarget{node: node}, ok
	}
	var parent parser.SyntaxNode
	var ok bool
	if target.viewParent.Valid() {
		parent, ok = target.viewParent.Snapshot()
	} else {
		parent, ok = target.parent.Snapshot()
	}
	if !ok || target.childStart < 0 || target.childEnd < target.childStart {
		return frozenMatchTarget{}, false
	}
	children := parent.Children()
	if target.childEnd > len(children) || !validSequenceTarget(target, parent, children) {
		return frozenMatchTarget{}, false
	}
	anchor := sequenceAnchor(parent.Range(), children, target.childStart)
	return frozenMatchTarget{kind: target.kind, field: target.field, children: children[target.childStart:target.childEnd], anchor: anchor, sequence: true}, true
}

func validSequenceTarget(target MatchTarget, parent parser.SyntaxNode, children []parser.SyntaxNode) bool {
	if sequenceParent(target.kind, parent.Kind()) {
		return true
	}
	if target.kind != "list_sequence" || target.childStart == target.childEnd {
		return false
	}
	found := false
	for _, child := range children[target.childStart:target.childEnd] {
		if child.IsNamed() && child.FieldName() == target.field && repeatedGrammarPositionKinds(parent.Kind(), target.field, children) {
			found = true
			continue
		}
		if child.IsNamed() && !child.IsExtra() && child.Kind() != "comment" {
			return false
		}
	}
	return found
}
func canonicalSequenceChildren(pattern []templateChild, children []parser.SyntaxNode) []parser.SyntaxNode {
	if len(children) == 0 {
		return children
	}
	last := children[len(children)-1]
	if !last.IsNamed() && listSeparator(last.Kind()) {
		patternHasSeparator := len(pattern) > 0 && pattern[len(pattern)-1].node != nil && pattern[len(pattern)-1].node.kind == last.Kind()
		if !patternHasSeparator {
			return children[:len(children)-1]
		}
	}
	return children
}

func sequenceParent(kind, parent string) bool {
	return kind == "statement_sequence" && parent == "statement_list" ||
		kind == "declaration_sequence" && parent == "source_file"
}

func sequenceAnchor(parent parser.Range, children []parser.SyntaxNode, index int) parser.Range {
	if index < 0 || index > len(children) {
		return zeroRange(parent, true)
	}
	position := parent.StartByte
	if index < len(children) {
		position = children[index].Range().StartByte
	} else if index > 0 {
		position = children[index-1].Range().EndByte
	}
	next := index
	for next < len(children) && omitMatchNode(children[next]) {
		next++
	}
	previous := index - 1
	for previous >= 0 && omitMatchNode(children[previous]) {
		previous--
	}
	if next < len(children) {
		nextRange := children[next].Range()
		if previous >= 0 {
			previousRange := children[previous].Range()
			if position-previousRange.EndByte < nextRange.StartByte-position {
				return zeroRange(previousRange, false)
			}
		}
		return zeroRange(nextRange, true)
	}
	if previous >= 0 {
		return zeroRange(children[previous].Range(), false)
	}
	return zeroRange(parent, true)
}

func matchRootSlot(slot *templateSlot, target frozenMatchTarget, state map[VariableID]*bindingValue, budget *evaluationBudget) (map[VariableID]*bindingValue, parser.Range, bool) {
	if !takeMatchStep(budget) {
		return nil, parser.Range{}, false
	}
	if target.sequence {
		nodes := significantNodes(target.children)
		field := target.field
		if target.kind != "list_sequence" {
			field = ""
		}
		elems := listElements(nodes, field)
		return bindNodes(slot, elems, state, target.anchor, budget)
	}
	if !target.node.Valid() || target.node.HasError() {
		return nil, parser.Range{}, false
	}
	return bindNodes(slot, []parser.SyntaxNode{target.node}, state, zeroRange(target.node.Range(), true), budget)
}

func sequenceKindForContext(context SnippetContext) string {
	switch context {
	case SnippetContextStatementList:
		return "statement_sequence"
	case SnippetContextDeclarationList:
		return "declaration_sequence"
	default:
		return ""
	}
}

// These sets are the transitive named subtype sets from tree-sitter-go v0.25.0
// node-types.json. Keeping them pinned beside repeatedGoFields makes root-slot
// category checks deterministic and avoids reparsing on the matching hot path.
var goExpressionKinds = kindSet(
	"binary_expression", "call_expression", "composite_literal", "false", "float_literal",
	"func_literal", "identifier", "imaginary_literal", "index_expression", "int_literal",
	"interpreted_string_literal", "iota", "nil", "parenthesized_expression", "raw_string_literal",
	"rune_literal", "selector_expression", "slice_expression", "true", "type_assertion_expression",
	"type_conversion_expression", "type_instantiation_expression", "unary_expression",
)

var goTypeKinds = kindSet(
	"array_type", "channel_type", "function_type", "generic_type", "interface_type", "map_type",
	"negated_type", "pointer_type", "qualified_type", "slice_type", "struct_type", "type_identifier",
	"parenthesized_type",
)

var goStatementKinds = kindSet(
	"assignment_statement", "dec_statement", "expression_statement", "inc_statement", "send_statement",
	"short_var_declaration", "block", "break_statement", "continue_statement", "defer_statement",
	"empty_statement", "expression_switch_statement", "fallthrough_statement", "for_statement",
	"go_statement", "goto_statement", "if_statement", "labeled_statement", "return_statement",
	"select_statement", "type_switch_statement", "var_declaration", "const_declaration", "type_declaration",
)

var goDeclarationKinds = kindSet(
	"import_declaration", "const_declaration", "type_declaration", "var_declaration",
	"function_declaration", "method_declaration",
)

func kindSet(kinds ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		out[kind] = struct{}{}
	}
	return out
}

func rootCategoryAccepts(context SnippetContext, kind string) bool {
	var set map[string]struct{}
	switch context {
	case SnippetContextExpression:
		set = goExpressionKinds
	case SnippetContextType:
		set = goTypeKinds
	case SnippetContextStatement, SnippetContextStatementList:
		set = goStatementKinds
	case SnippetContextDeclaration, SnippetContextDeclarationList:
		set = goDeclarationKinds
	case SnippetContextFile:
		return kind == "source_file"
	default:
		return false
	}
	_, ok := set[kind]
	return ok
}

func sequenceCategoryAccepts(context SnippetContext, nodes []parser.SyntaxNode) bool {
	significant := significantNodes(nodes)
	named := 0
	for _, node := range significant {
		if node.IsNamed() {
			named++
			if !rootCategoryAccepts(context, node.Kind()) {
				return false
			}
		}
	}
	return named > 0 || len(significant) == 0
}

func matchNode(t *templateNode, n parser.SyntaxNode, state map[VariableID]*bindingValue, budget *evaluationBudget) (map[VariableID]*bindingValue, parser.Range, bool) {
	if !takeMatchStep(budget) {
		return nil, parser.Range{}, false
	}
	if t == nil || !n.Valid() || n.HasError() || t.kind != n.Kind() || t.named != n.IsNamed() {
		return nil, parser.Range{}, false
	}
	children := significantNodes(n.Children())
	if len(t.children) == 0 {
		if len(children) != 0 || t.token != n.Text() {
			return nil, parser.Range{}, false
		}
		r, _, ok := normalizeNode(n)
		return state, r, ok
	}
	if len(children) == 0 {
		return nil, parser.Range{}, false
	}
	out, ok := matchChildren(t.children, children, 0, 0, state, zeroRange(children[0].Range(), true), budget)
	if !ok {
		return nil, parser.Range{}, false
	}
	r, _, normalized := normalizeNode(n)
	return out, r, normalized
}

func matchChildren(pattern []templateChild, candidates []parser.SyntaxNode, pi, ci int, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, bool) {
	if !takeMatchStep(budget) {
		return nil, false
	}
	if pi == len(pattern) {
		return state, ci == len(candidates)
	}
	child := pattern[pi]
	if child.slot == nil {
		return matchLiteralChild(pattern, candidates, pi, ci, state, anchor, budget)
	}
	if child.slot.cardinality == SlotOne {
		return matchSingleSlotChild(pattern, candidates, pi, ci, state, anchor, budget)
	}
	return matchRepeatedSlotChild(pattern, candidates, pi, ci, state, anchor, budget)
}

func matchLiteralChild(pattern []templateChild, candidates []parser.SyntaxNode, pi, ci int, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, bool) {
	if result, matched := matchConcreteChild(pattern, candidates, pi, ci, state, anchor, budget); matched {
		return result, true
	}
	if result, matched := matchAbsentOptionalChild(pattern, candidates, pi, ci, state, anchor, budget); matched {
		return result, true
	}
	if commaBeforeRepeatedSlot(pattern, pi) {
		return matchChildren(pattern, candidates, pi+1, ci, cloneBindingMap(state), anchor, budget)
	}
	return nil, false
}

func matchConcreteChild(pattern []templateChild, candidates []parser.SyntaxNode, pi, ci int, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, bool) {
	child := pattern[pi]
	if ci >= len(candidates) || child.field != candidates[ci].FieldName() {
		return nil, false
	}
	next, _, ok := matchNode(child.node, candidates[ci], cloneBindingMap(state), budget)
	if !ok {
		return nil, false
	}
	return matchChildren(pattern, candidates, pi+1, ci+1, next, anchor, budget)
}

func matchAbsentOptionalChild(pattern []templateChild, candidates []parser.SyntaxNode, pi, ci int, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, bool) {
	// Go omits a small number of optional, punctuation-free list wrappers
	// entirely when they are empty. A template wrapper containing only one
	// repeated slot therefore matches that absence and binds the slot at the
	// next concrete token (or after the previous one at the end).
	slot, omittable := absentOptionalListSlot(pattern[pi].node)
	if !omittable {
		return nil, false
	}
	next, _, bound := bindNodes(slot, nil, cloneBindingMap(state), anchorAt(candidates, ci, anchor), budget)
	if !bound {
		return nil, false
	}
	return matchChildren(pattern, candidates, pi+1, ci, next, anchor, budget)
}

func commaBeforeRepeatedSlot(pattern []templateChild, pi int) bool {
	// A comma authored next to a repeated grammar value belongs to the list,
	// so it disappears when that adjacent value is empty. Semicolons are never
	// optional: explicit and automatic statement termination stay distinct.
	return pattern[pi].node != nil && pattern[pi].node.kind == "," && pi+1 < len(pattern) &&
		pattern[pi+1].slot != nil && pattern[pi+1].slot.cardinality == SlotMany
}

func matchSingleSlotChild(pattern []templateChild, candidates []parser.SyntaxNode, pi, ci int, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, bool) {
	child := pattern[pi]
	if ci >= len(candidates) || !slotElement(candidates[ci], child.field) {
		return nil, false
	}
	next, _, ok := bindNodes(child.slot, []parser.SyntaxNode{candidates[ci]}, cloneBindingMap(state), anchorAt(candidates, ci, anchor), budget)
	if !ok {
		return nil, false
	}
	return matchChildren(pattern, candidates, pi+1, ci+1, next, anchor, budget)
}

func matchRepeatedSlotChild(pattern []templateChild, candidates []parser.SyntaxNode, pi, ci int, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, bool) {
	child := pattern[pi]
	ends := repeatedSlotEnds(candidates, ci, child.field)
	for i := len(ends) - 1; i >= 0; i-- {
		end := ends[i]
		segment := candidates[ci:end]
		elems := listElements(segment, child.field)
		next, _, ok := bindNodes(child.slot, elems, cloneBindingMap(state), anchorAt(candidates, ci, anchor), budget)
		if !ok {
			continue
		}
		if result, matched := matchChildren(pattern, candidates, pi+1, end, next, anchor, budget); matched {
			return result, true
		}
	}
	return matchRepeatedSlotWithoutFollowingComma(pattern, candidates, pi, ci, state, anchor, budget)
}

func repeatedSlotEnds(candidates []parser.SyntaxNode, start int, field string) []int {
	// A repeated grammar value consists of named/fielded elements and may include
	// unnamed separators between them. Try the longest valid prefix first.
	ends := []int{start}
	for end := start + 1; end <= len(candidates); end++ {
		segment := candidates[start:end]
		if len(listElements(segment, field)) > 0 && slotSegment(segment, field) {
			ends = append(ends, end)
		}
	}
	return ends
}

func matchRepeatedSlotWithoutFollowingComma(pattern []templateChild, candidates []parser.SyntaxNode, pi, ci int, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, bool) {
	// For a leading or middle empty list value, omit its following authored comma.
	if pi+1 >= len(pattern) || pattern[pi+1].slot != nil || pattern[pi+1].node == nil || pattern[pi+1].node.kind != "," {
		return nil, false
	}
	next, _, ok := bindNodes(pattern[pi].slot, nil, cloneBindingMap(state), anchorAt(candidates, ci, anchor), budget)
	if !ok {
		return nil, false
	}
	return matchChildren(pattern, candidates, pi+2, ci, next, anchor, budget)
}

// Go's grammar represents these two optional, delimiter-free repeated values
// with named containers, but emits no container node when their list is empty.
// Other optional nodes either are not list containers or carry authored syntax.
func absentOptionalListSlot(n *templateNode) (*templateSlot, bool) {
	if n == nil || !n.named || len(n.children) != 1 {
		return nil, false
	}
	if n.kind != "statement_list" && n.kind != "expression_list" {
		return nil, false
	}
	child := n.children[0]
	if child.field != "" || child.node != nil || child.slot == nil || child.slot.cardinality != SlotMany {
		return nil, false
	}
	return child.slot, true
}

func slotSegment(nodes []parser.SyntaxNode, field string) bool {
	seen := false
	for _, n := range nodes {
		if slotElement(n, field) {
			seen = true
			continue
		}
		if n.IsNamed() || !seen || !listSeparator(n.Kind()) {
			return false
		}
	}
	return seen
}

func listSeparator(kind string) bool {
	return kind == "," || kind == ";" || kind == "|"
}

func slotElement(n parser.SyntaxNode, field string) bool {
	return n.IsNamed() && n.FieldName() == field
}

func listElements(nodes []parser.SyntaxNode, field string) []parser.SyntaxNode {
	out := make([]parser.SyntaxNode, 0, len(nodes))
	for _, n := range nodes {
		if slotElement(n, field) {
			out = append(out, n)
		}
	}
	return out
}

func bindNodes(slot *templateSlot, nodes []parser.SyntaxNode, state map[VariableID]*bindingValue, anchor parser.Range, budget *evaluationBudget) (map[VariableID]*bindingValue, parser.Range, bool) {
	if !takeMatchStep(budget) {
		return nil, parser.Range{}, false
	}
	if slot.cardinality == SlotOne && (len(nodes) != 1 || !nodes[0].IsNamed()) {
		return nil, parser.Range{}, false
	}
	ranges, structural, ok := normalizeBindingNodes(nodes)
	if !ok {
		return nil, parser.Range{}, false
	}
	// Equality is element-wise normalized structure only. Separators are syntax
	// consumed by matching, never members of a repeated binding.
	equality := append([]normalizedNode(nil), structural...)
	rng := rangesSpan(ranges, anchor)
	if slot.ref.Anonymous {
		return state, rng, true
	}
	kind := bindingKind(slot)
	if old, exists := state[slot.ref.ID]; exists {
		if old.kind != kind || !reflect.DeepEqual(old.equality, equality) {
			return nil, parser.Range{}, false
		}
		return state, rng, true
	}
	text := ""
	if len(nodes) > 0 {
		text = nodes[0].TextRange(rng.StartByte, rng.EndByte)
	}
	sourceRanges, sourceKinds := bindingSourceDetails(nodes)
	state[slot.ref.ID] = &bindingValue{variable: slot.ref, kind: kind, rng: rng, ranges: ranges, structural: structural, equality: equality, sourceRanges: sourceRanges, sourceKinds: sourceKinds, text: text}
	return state, rng, true
}

func normalizeBindingNodes(nodes []parser.SyntaxNode) ([]parser.Range, []normalizedNode, bool) {
	ranges := make([]parser.Range, len(nodes))
	for i, node := range nodes {
		if !node.Valid() || node.HasError() {
			return nil, nil, false
		}
		rng, _, ok := normalizeNode(node)
		if !ok {
			return nil, nil, false
		}
		ranges[i] = rng
	}
	structural := make([]normalizedNode, len(nodes))
	for i, node := range nodes {
		_, normalized, ok := normalizeNode(node)
		if !ok {
			return nil, nil, false
		}
		structural[i] = normalized
	}
	return ranges, structural, true
}

func bindingKind(slot *templateSlot) BindingKind {
	if slot.cardinality == SlotOne {
		return BindingNode
	}
	return BindingList
}

func bindingSourceDetails(nodes []parser.SyntaxNode) ([]parser.Range, []string) {
	ranges := make([]parser.Range, len(nodes))
	kinds := make([]string, len(nodes))
	for i := range nodes {
		ranges[i] = nodes[i].Range()
		kinds[i] = nodes[i].Kind()
	}
	return ranges, kinds
}

func normalizeNode(n parser.SyntaxNode) (parser.Range, normalizedNode, bool) {
	if !n.Valid() || n.HasError() || omitMatchNode(n) {
		return parser.Range{}, normalizedNode{}, false
	}
	children := significantNodes(n.Children())
	out := normalizedNode{kind: n.Kind(), named: n.IsNamed()}
	if len(children) == 0 {
		out.lexeme = strings.Clone(n.Text())
		return n.Range(), out, true
	}
	var first, last parser.Range
	for i, child := range children {
		r, normalized, ok := normalizeNode(child)
		if !ok {
			return parser.Range{}, normalizedNode{}, false
		}
		if i == 0 {
			first = r
		}
		last = r
		out.children = append(out.children, normalized)
	}
	r := n.Range()
	r.StartByte, r.Start = first.StartByte, first.Start
	r.EndByte, r.End = last.EndByte, last.End
	return r, out, true
}

func significantNodes(nodes []parser.SyntaxNode) []parser.SyntaxNode {
	out := make([]parser.SyntaxNode, 0, len(nodes))
	for _, n := range nodes {
		if !omitMatchNode(n) {
			out = append(out, n)
		}
	}
	return out
}

func omitMatchNode(n parser.SyntaxNode) bool {
	if !n.Valid() || n.IsExtra() || n.Kind() == "comment" {
		return true
	}
	r := n.Range()
	return !n.IsNamed() && r.StartByte == r.EndByte
}

func cloneBindingMap(in map[VariableID]*bindingValue) map[VariableID]*bindingValue {
	out := make(map[VariableID]*bindingValue, len(in))
	for id, value := range in {
		out[id] = value
	}
	return out
}

func anchorAt(nodes []parser.SyntaxNode, index int, fallback parser.Range) parser.Range {
	if index < len(nodes) {
		return zeroRange(nodes[index].Range(), true)
	}
	if index > 0 {
		return zeroRange(nodes[index-1].Range(), false)
	}
	return fallback
}

func rangesSpan(ranges []parser.Range, anchor parser.Range) parser.Range {
	if len(ranges) == 0 {
		return anchor
	}
	r := ranges[0]
	r.EndByte, r.End = ranges[len(ranges)-1].EndByte, ranges[len(ranges)-1].End
	return r
}

func nodesRange(nodes []parser.SyntaxNode, anchor parser.Range) parser.Range {
	if len(nodes) == 0 {
		return anchor
	}
	first, _, ok1 := normalizeNode(nodes[0])
	last, _, ok2 := normalizeNode(nodes[len(nodes)-1])
	if !ok1 || !ok2 {
		return parser.Range{}
	}
	first.EndByte, first.End = last.EndByte, last.End
	return first
}

func zeroRange(r parser.Range, atStart bool) parser.Range {
	if atStart {
		return parser.Range{StartByte: r.StartByte, EndByte: r.StartByte, Start: r.Start, End: r.Start}
	}
	return parser.Range{StartByte: r.EndByte, EndByte: r.EndByte, Start: r.End, End: r.End}
}
