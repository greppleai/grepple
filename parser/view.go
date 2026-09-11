package parser

import (
	"errors"
	"sync"
	"sync/atomic"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ErrDocumentClosed indicates that a read view was requested after its document closed.
var ErrDocumentClosed = errors.New("parser document is closed")

// DocumentView provides lock-free, read-only access while a Document.Read callback
// is active. A view and all ViewNodes derived from it become invalid when the
// callback returns and must not be retained.
type DocumentView struct {
	doc   *Document
	state *documentViewState
}

type documentViewState struct {
	active    atomic.Bool
	mu        sync.Mutex
	snapshots map[viewSnapshotKey]SyntaxNode
}

type viewSnapshotKey struct {
	id    uintptr
	field string
}

// ViewNode is a callback-scoped syntax node. Unlike Node, its accessors do not
// acquire the document lock because Document.Read holds it for the whole view.
type ViewNode struct {
	view      DocumentView
	raw       *sitter.Node
	fieldName string
}

// Read holds one document read lock while visit traverses a stable syntax tree.
// The callback must access the document through its view: it must not call other
// Document methods or retain the view or its nodes after returning.
func (d *Document) Read(visit func(DocumentView) error) error {
	if d == nil || visit == nil {
		return ErrDocumentClosed
	}
	d.mu.RLock()
	if d.tree == nil {
		d.mu.RUnlock()
		return ErrDocumentClosed
	}
	state := &documentViewState{snapshots: make(map[viewSnapshotKey]SyntaxNode)}
	state.active.Store(true)
	view := DocumentView{doc: d, state: state}
	defer func() {
		state.active.Store(false)
		d.mu.RUnlock()
	}()
	return visit(view)
}

// Language returns the canonical language identifier while the view is active.
func (v DocumentView) Language() string {
	if !v.valid() {
		return ""
	}
	return v.doc.language
}

// Source returns the immutable document source while the view is active.
func (v DocumentView) Source() string {
	if !v.valid() {
		return ""
	}
	return v.doc.source
}

// Root returns the syntax-tree root while the view is active.
func (v DocumentView) Root() ViewNode {
	if !v.valid() {
		return ViewNode{}
	}
	return ViewNode{view: v, raw: v.doc.tree.RootNode()}
}

func (v DocumentView) valid() bool {
	return v.doc != nil && v.state != nil && v.state.active.Load()
}

// Valid reports whether the view and node are still active.
func (n ViewNode) Valid() bool { return n.raw != nil && n.view.valid() }

// IsValid is an alias for Valid.
func (n ViewNode) IsValid() bool { return n.Valid() }

// Kind returns the grammar node kind.
func (n ViewNode) Kind() string {
	if !n.Valid() {
		return ""
	}
	return n.raw.Kind()
}

// IsNamed reports whether the grammar marks the node as named.
func (n ViewNode) IsNamed() bool { return n.Valid() && n.raw.IsNamed() }

// IsExtra reports whether the grammar marks the node as extra syntax.
func (n ViewNode) IsExtra() bool { return n.Valid() && n.raw.IsExtra() }

// IsError reports whether the node is an error-recovery node.
func (n ViewNode) IsError() bool { return n.Valid() && n.raw.IsError() }

// IsMissing reports whether the parser inserted the node during recovery.
func (n ViewNode) IsMissing() bool { return n.Valid() && n.raw.IsMissing() }

// HasError reports whether the node or a descendant contains recovery syntax.
func (n ViewNode) HasError() bool { return n.Valid() && n.raw.HasError() }

// Text returns the exact source text covered by the node.
func (n ViewNode) Text() string {
	if !n.Valid() {
		return ""
	}
	return n.TextRange(int(n.raw.StartByte()), int(n.raw.EndByte()))
}

// Source is an alias for Text.
func (n ViewNode) Source() string { return n.Text() }

// TextRange returns source text for a valid half-open byte range.
func (n ViewNode) TextRange(startByte, endByte int) string {
	if !n.Valid() || startByte < 0 || endByte < startByte || endByte > len(n.view.doc.source) {
		return ""
	}
	return n.view.doc.source[startByte:endByte]
}

// Range returns the node's half-open source range.
func (n ViewNode) Range() Range {
	if !n.Valid() {
		return Range{}
	}
	return n.view.doc.nodeRangeLocked(n.raw)
}

// Parent returns the parent node or an invalid node at the root.
func (n ViewNode) Parent() ViewNode {
	if !n.Valid() {
		return ViewNode{}
	}
	parent := n.raw.Parent()
	if parent == nil {
		return ViewNode{}
	}
	return ViewNode{view: n.view, raw: parent}
}

// ChildCount returns the number of named and unnamed direct children.
func (n ViewNode) ChildCount() int {
	if !n.Valid() {
		return 0
	}
	return int(n.raw.ChildCount())
}

// NamedChildCount returns the number of named direct children.
func (n ViewNode) NamedChildCount() int {
	if !n.Valid() {
		return 0
	}
	return int(n.raw.NamedChildCount())
}

// Child returns one named or unnamed child.
func (n ViewNode) Child(index int) ViewNode {
	if !n.Valid() || index < 0 || index >= int(n.raw.ChildCount()) {
		return ViewNode{}
	}
	child := n.raw.Child(uint(index))
	if child == nil {
		return ViewNode{}
	}
	return ViewNode{view: n.view, raw: child, fieldName: n.raw.FieldNameForChild(uint32(index))}
}

// NamedChild returns one named child.
func (n ViewNode) NamedChild(index int) ViewNode {
	if !n.Valid() || index < 0 || index >= int(n.raw.NamedChildCount()) {
		return ViewNode{}
	}
	child := n.raw.NamedChild(uint(index))
	if child == nil {
		return ViewNode{}
	}
	return ViewNode{view: n.view, raw: child, fieldName: n.raw.FieldNameForNamedChild(uint32(index))}
}

// Children returns all direct children in source order.
func (n ViewNode) Children() []ViewNode {
	if !n.Valid() {
		return nil
	}
	children := make([]ViewNode, 0, n.raw.ChildCount())
	cursor := n.raw.Walk()
	defer cursor.Close()
	if !cursor.GotoFirstChild() {
		return children
	}
	for {
		children = append(children, ViewNode{view: n.view, raw: cursor.Node(), fieldName: cursor.FieldName()})
		if !cursor.GotoNextSibling() {
			return children
		}
	}
}

// NamedChildren returns named direct children in source order.
func (n ViewNode) NamedChildren() []ViewNode {
	children := n.Children()
	result := make([]ViewNode, 0, len(children))
	for _, child := range children {
		if child.IsNamed() {
			result = append(result, child)
		}
	}
	return result
}

// ChildByFieldName returns the child assigned to name.
func (n ViewNode) ChildByFieldName(name string) ViewNode {
	if !n.Valid() {
		return ViewNode{}
	}
	child := n.raw.ChildByFieldName(name)
	if child == nil {
		return ViewNode{}
	}
	return ViewNode{view: n.view, raw: child, fieldName: name}
}

// FieldName returns this node's grammar field in its parent.
func (n ViewNode) FieldName() string {
	if !n.Valid() {
		return ""
	}
	return n.fieldName
}

// FieldNameForChild returns the grammar field for an indexed child.
func (n ViewNode) FieldNameForChild(index int) string {
	if !n.Valid() || index < 0 || index >= int(n.raw.ChildCount()) {
		return ""
	}
	return n.raw.FieldNameForChild(uint32(index))
}

// Snapshot copies this node and its descendants into an immutable syntax tree.
func (n ViewNode) Snapshot() (SyntaxNode, bool) {
	if !n.Valid() {
		return SyntaxNode{}, false
	}
	return n.snapshot(), true
}

func (n ViewNode) snapshot() SyntaxNode {
	key := viewSnapshotKey{id: n.raw.Id(), field: n.fieldName}
	n.view.state.mu.Lock()
	cached, ok := n.view.state.snapshots[key]
	n.view.state.mu.Unlock()
	if ok {
		return cached
	}
	rng := n.view.doc.nodeRangeLocked(n.raw)
	snapshot := SyntaxNode{
		valid: true, kind: n.raw.Kind(), fieldName: n.fieldName, source: n.view.doc.source,
		named: n.raw.IsNamed(), extra: n.raw.IsExtra(), hasError: n.raw.HasError(), rng: rng,
	}
	if rng.StartByte >= 0 && rng.EndByte >= rng.StartByte && rng.EndByte <= len(n.view.doc.source) {
		snapshot.text = n.view.doc.source[rng.StartByte:rng.EndByte]
	}
	snapshot.children = make([]SyntaxNode, 0, n.raw.ChildCount())
	for i := 0; i < int(n.raw.ChildCount()); i++ {
		child := n.Child(i)
		if child.Valid() {
			snapshot.children = append(snapshot.children, child.snapshot())
		}
	}
	n.view.state.mu.Lock()
	if cached, ok = n.view.state.snapshots[key]; ok {
		snapshot = cached
	} else {
		n.view.state.snapshots[key] = snapshot
	}
	n.view.state.mu.Unlock()
	return snapshot
}

// WalkOptions bounds a syntax walk. Zero limits are unbounded.
type WalkOptions struct {
	MaxDepth int
	MaxNodes int
}

// WalkResult reports completed visits and whether a configured bound stopped traversal.
type WalkResult struct {
	Visited   int
	Truncated bool
}

// WalkNamedView visits root and all named descendants in pre-order.
func WalkNamedView(root ViewNode, visit func(ViewNode)) {
	if visit == nil {
		return
	}
	WalkNamedViewBounded(root, WalkOptions{}, func(node ViewNode, _ int) bool {
		visit(node)
		return true
	})
}

type viewWalkItem struct {
	node  ViewNode
	depth int
}

// WalkNamedViewBounded visits named nodes in pre-order. Returning false from
// visit prunes that node's descendants without stopping sibling traversal.
func WalkNamedViewBounded(root ViewNode, options WalkOptions, visit func(ViewNode, int) bool) WalkResult {
	if !root.Valid() || visit == nil {
		return WalkResult{}
	}
	stack := []viewWalkItem{{node: root, depth: 1}}
	result := WalkResult{}
	for len(stack) > 0 {
		if options.MaxNodes > 0 && result.Visited >= options.MaxNodes {
			result.Truncated = true
			break
		}
		index := len(stack) - 1
		current := stack[index]
		stack = stack[:index]
		result.Visited++
		if !visit(current.node, current.depth) {
			continue
		}
		var truncated bool
		stack, truncated = appendViewWalkChildren(stack, current, options.MaxDepth)
		if truncated {
			result.Truncated = true
		}
	}
	return result
}

func appendViewWalkChildren(stack []viewWalkItem, current viewWalkItem, maxDepth int) ([]viewWalkItem, bool) {
	children := current.node.NamedChildren()
	if maxDepth > 0 && current.depth >= maxDepth {
		return stack, len(children) > 0
	}
	for i := len(children) - 1; i >= 0; i-- {
		stack = append(stack, viewWalkItem{node: children[i], depth: current.depth + 1})
	}
	return stack, false
}
