package parser

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Position is a one-based source position. Column counts Unicode scalar values,
// not bytes. Positions at the end of a node are exclusive.
type Position struct {
	Line   int
	Column int
}

// Range identifies an exclusive byte and source-position range. Byte offsets
// are zero-based; line and column positions are one-based.
type Range struct {
	StartByte int
	EndByte   int
	Start     Position
	End       Position
}

// ParseDiagnostic describes an error-recovery node found in a parsed document.
type ParseDiagnostic struct {
	Message string
	Kind    string
	Range   Range
}

// Document owns a syntax tree and an immutable copy of its source. Call Close
// when it is no longer needed. A Document must not be copied after first use.
// Its methods and Nodes may safely be read concurrently with Close.
type Document struct {
	mu         sync.RWMutex
	language   string
	source     string
	lineStarts []int
	tree       *sitter.Tree
}

// Node is a language-neutral handle into a Document's syntax tree. It remains
// usable only until that document is closed. The zero value is an invalid node.
type Node struct {
	doc       *Document
	raw       *sitter.Node
	fieldName string
}

// SyntaxNode is an immutable syntax-tree snapshot. A snapshot remains usable
// after its Document is closed and is safe for concurrent use.
type SyntaxNode struct {
	valid     bool
	kind      string
	text      string
	source    string
	fieldName string
	named     bool
	extra     bool
	hasError  bool
	rng       Range
	children  []SyntaxNode
}

// Valid reports whether the snapshot represents a syntax node.
func (n SyntaxNode) Valid() bool { return n.valid }

// Kind returns the language grammar's node kind.
func (n SyntaxNode) Kind() string { return n.kind }

// Text returns the exact source text covered by the node.
func (n SyntaxNode) Text() string { return n.text }

// TextRange returns source text for a valid half-open byte range in the snapshot's document.
func (n SyntaxNode) TextRange(startByte, endByte int) string {
	if startByte < 0 || endByte < startByte || endByte > len(n.source) {
		return ""
	}
	return n.source[startByte:endByte]
}

// FieldName returns the node's grammar field in its parent, when present.
func (n SyntaxNode) FieldName() string { return n.fieldName }

// IsNamed reports whether the grammar marks the node as named.
func (n SyntaxNode) IsNamed() bool { return n.named }

// IsExtra reports whether the grammar marks the node as extra syntax.
func (n SyntaxNode) IsExtra() bool { return n.extra }

// HasError reports whether the node or any descendant contains recovery syntax.
func (n SyntaxNode) HasError() bool { return n.hasError }

// Range returns the node's half-open source range.
func (n SyntaxNode) Range() Range { return n.rng }

// Children returns an immutable copy of all children in concrete source order.
func (n SyntaxNode) Children() []SyntaxNode {
	return append([]SyntaxNode(nil), n.children...)
}

// Snapshot atomically copies a node and its descendants while holding the
// document's read lock. Close cannot invalidate any part of the copy.
func (n Node) Snapshot() (SyntaxNode, bool) {
	if n.doc == nil || n.raw == nil {
		return SyntaxNode{}, false
	}
	n.doc.mu.RLock()
	defer n.doc.mu.RUnlock()
	if n.doc.tree == nil {
		return SyntaxNode{}, false
	}
	return n.snapshotLocked(), true
}

func (n Node) snapshotLocked() SyntaxNode {
	r := n.doc.nodeRangeLocked(n.raw)
	out := SyntaxNode{
		valid: true, kind: n.raw.Kind(), fieldName: n.fieldName, source: n.doc.source,
		named: n.raw.IsNamed(), extra: n.raw.IsExtra(),
		hasError: n.raw.HasError(), rng: r,
	}
	if r.StartByte >= 0 && r.EndByte >= r.StartByte && r.EndByte <= len(n.doc.source) {
		out.text = n.doc.source[r.StartByte:r.EndByte]
	}
	out.children = make([]SyntaxNode, 0, n.raw.ChildCount())
	for i := uint(0); i < n.raw.ChildCount(); i++ {
		child := n.raw.Child(i)
		if child != nil {
			out.children = append(out.children, Node{doc: n.doc, raw: child, fieldName: n.raw.FieldNameForChild(uint32(i))}.snapshotLocked())
		}
	}
	return out
}

// ParseDocument parses UTF-8 content with a configured language. The returned
// document owns its source and tree independently of the pooled parser.
func ParseDocument(language string, content string) (*Document, error) {
	if !utf8.ValidString(content) {
		return nil, fmt.Errorf("source is not valid UTF-8")
	}
	adapter := adapterForLanguage(language)
	if adapter == nil || adapter.Grammar() == nil {
		return nil, fmt.Errorf("unsupported language %q", language)
	}
	owned := strings.Clone(content)
	tree, err := parseTree(adapter, owned)
	if err != nil {
		return nil, err
	}
	doc := &Document{language: adapter.ID(), source: owned, lineStarts: sourceLineStarts(owned), tree: tree}
	runtime.SetFinalizer(doc, func(d *Document) { d.Close() })
	return doc, nil
}

// Close releases the syntax tree and invalidates all Nodes from the document.
// It is safe to call more than once.
func (d *Document) Close() {
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
	d.language = ""
	d.source = ""
	d.lineStarts = nil
	runtime.SetFinalizer(d, nil)
}

// Language returns the configured language identifier, or empty after Close.
func (d *Document) Language() string {
	if d == nil {
		return ""
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return ""
	}
	return d.language
}

// Source returns the complete immutable source owned by the document.
func (d *Document) Source() string {
	if d == nil {
		return ""
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return ""
	}
	return d.source
}

// Text is an alias for Source.
func (d *Document) Text() string { return d.Source() }

// Root returns the document's syntax-tree root, or an invalid node after Close.
func (d *Document) Root() Node {
	if d == nil {
		return Node{}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return Node{}
	}
	return Node{doc: d, raw: d.tree.RootNode()}
}

// ParseDiagnostics returns error and missing nodes in deterministic source-tree
// order. Valid recovery trees are still returned by ParseDocument.
func (d *Document) ParseDiagnostics() []ParseDiagnostic {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tree == nil {
		return nil
	}
	var diagnostics []ParseDiagnostic
	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node.IsError() || node.IsMissing() {
			message := "syntax error"
			if node.IsMissing() {
				message = "missing " + node.Kind()
			}
			diagnostics = append(diagnostics, ParseDiagnostic{
				Message: message,
				Kind:    node.Kind(),
				Range:   d.nodeRangeLocked(node),
			})
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			if child := node.Child(i); child != nil {
				walk(child)
			}
		}
	}
	walk(d.tree.RootNode())
	return diagnostics
}

// Diagnostics is an alias for ParseDiagnostics.
func (d *Document) Diagnostics() []ParseDiagnostic { return d.ParseDiagnostics() }

// Valid reports whether the node refers to an open document's syntax tree.
func (n Node) Valid() bool {
	if n.doc == nil || n.raw == nil {
		return false
	}
	n.doc.mu.RLock()
	defer n.doc.mu.RUnlock()
	return n.doc.tree != nil
}

// IsValid is an alias for Valid.
func (n Node) IsValid() bool { return n.Valid() }

// Kind returns the language grammar's node kind, or empty for an invalid node.
func (n Node) Kind() string {
	var value string
	n.read(func() { value = n.raw.Kind() })
	return value
}

// IsNamed reports whether the grammar marks the node as named.
func (n Node) IsNamed() bool {
	var value bool
	n.read(func() { value = n.raw.IsNamed() })
	return value
}

// IsExtra reports whether the grammar marks the node as extra syntax.
func (n Node) IsExtra() bool {
	var value bool
	n.read(func() { value = n.raw.IsExtra() })
	return value
}

// IsError reports whether the node is an error-recovery node.
func (n Node) IsError() bool {
	var value bool
	n.read(func() { value = n.raw.IsError() })
	return value
}

// IsMissing reports whether the parser inserted the node during recovery.
func (n Node) IsMissing() bool {
	var value bool
	n.read(func() { value = n.raw.IsMissing() })
	return value
}

// HasError reports whether the node or any descendant contains recovery syntax.
func (n Node) HasError() bool {
	var value bool
	n.read(func() { value = n.raw.HasError() })
	return value
}

// Text returns the exact source bytes covered by the node.
func (n Node) Text() string {
	var value string
	n.read(func() {
		start, end := int(n.raw.StartByte()), int(n.raw.EndByte())
		if start >= 0 && end >= start && end <= len(n.doc.source) {
			value = n.doc.source[start:end]
		}
	})
	return value
}

// Source is an alias for Text.
func (n Node) Source() string { return n.Text() }

// TextRange returns the source bytes in the half-open byte range. The range
// must belong to the same document and be within its immutable source.
func (n Node) TextRange(startByte, endByte int) string {
	var value string
	n.read(func() {
		if startByte >= 0 && endByte >= startByte && endByte <= len(n.doc.source) {
			value = n.doc.source[startByte:endByte]
		}
	})
	return value
}

// Range returns the node's half-open source range, or zero for an invalid node.
func (n Node) Range() Range {
	var value Range
	n.read(func() { value = n.doc.nodeRangeLocked(n.raw) })
	return value
}

// Parent returns the parent node, or an invalid node at the root or after Close.
func (n Node) Parent() Node {
	var value Node
	n.read(func() {
		if parent := n.raw.Parent(); parent != nil {
			value = Node{doc: n.doc, raw: parent}
		}
	})
	return value
}

// ChildCount returns the number of named and unnamed direct children.
func (n Node) ChildCount() int {
	count := 0
	n.read(func() { count = int(n.raw.ChildCount()) })
	return count
}

// NamedChildCount returns the number of named direct children.
func (n Node) NamedChildCount() int {
	count := 0
	n.read(func() { count = int(n.raw.NamedChildCount()) })
	return count
}

// Child returns the indexed named or unnamed child, or an invalid node when out of range.
func (n Node) Child(index int) Node {
	var value Node
	n.read(func() {
		if index < 0 || index >= int(n.raw.ChildCount()) {
			return
		}
		if child := n.raw.Child(uint(index)); child != nil {
			value = Node{doc: n.doc, raw: child, fieldName: n.raw.FieldNameForChild(uint32(index))}
		}
	})
	return value
}

// NamedChild returns the indexed named child, or an invalid node when out of range.
func (n Node) NamedChild(index int) Node {
	var value Node
	n.read(func() {
		if index < 0 || index >= int(n.raw.NamedChildCount()) {
			return
		}
		if child := n.raw.NamedChild(uint(index)); child != nil {
			value = Node{doc: n.doc, raw: child, fieldName: n.raw.FieldNameForNamedChild(uint32(index))}
		}
	})
	return value
}

// Children returns all children in concrete source order, including unnamed
// punctuation and keywords.
func (n Node) Children() []Node {
	var result []Node
	n.read(func() {
		result = make([]Node, 0, n.raw.ChildCount())
		cursor := n.raw.Walk()
		defer cursor.Close()
		if !cursor.GotoFirstChild() {
			return
		}
		for {
			result = append(result, Node{doc: n.doc, raw: cursor.Node(), fieldName: cursor.FieldName()})
			if !cursor.GotoNextSibling() {
				return
			}
		}
	})
	return result
}

// NamedChildren returns named children in source order.
func (n Node) NamedChildren() []Node {
	var result []Node
	n.read(func() {
		result = make([]Node, 0, n.raw.NamedChildCount())
		cursor := n.raw.Walk()
		defer cursor.Close()
		if !cursor.GotoFirstChild() {
			return
		}
		for {
			child := cursor.Node()
			if child.IsNamed() {
				result = append(result, Node{doc: n.doc, raw: child, fieldName: cursor.FieldName()})
			}
			if !cursor.GotoNextSibling() {
				return
			}
		}
	})
	return result
}

// ChildByFieldName returns the child assigned to name, or an invalid node when absent.
func (n Node) ChildByFieldName(name string) Node {
	var value Node
	n.read(func() {
		if child := n.raw.ChildByFieldName(name); child != nil {
			value = Node{doc: n.doc, raw: child, fieldName: name}
		}
	})
	return value
}

// FieldName returns this node's grammar field in the parent, when the handle
// was obtained through a child operation.
func (n Node) FieldName() string {
	if !n.Valid() {
		return ""
	}
	return n.fieldName
}

// FieldNameForChild returns the grammar field for the indexed child, when present.
func (n Node) FieldNameForChild(index int) string {
	var value string
	n.read(func() {
		if index >= 0 && index < int(n.raw.ChildCount()) {
			value = n.raw.FieldNameForChild(uint32(index))
		}
	})
	return value
}

func (n Node) read(read func()) {
	if n.doc == nil || n.raw == nil {
		return
	}
	n.doc.mu.RLock()
	defer n.doc.mu.RUnlock()
	if n.doc.tree != nil {
		read()
	}
}

func (d *Document) nodeRangeLocked(node *sitter.Node) Range {
	start, end := int(node.StartByte()), int(node.EndByte())
	return Range{
		StartByte: start,
		EndByte:   end,
		Start:     sourcePosition(d.source, d.lineStarts, start),
		End:       sourcePosition(d.source, d.lineStarts, end),
	}
}

func sourceLineStarts(source string) []int {
	starts := []int{0}
	for index := 0; index < len(source); index++ {
		if source[index] == '\n' {
			starts = append(starts, index+1)
		}
	}
	return starts
}

func sourcePosition(source string, lineStarts []int, offset int) Position {
	if offset < 0 || offset > len(source) || len(lineStarts) == 0 {
		return Position{}
	}
	lineIndex := sort.Search(len(lineStarts), func(index int) bool {
		return lineStarts[index] > offset
	}) - 1
	lineStart := lineStarts[lineIndex]
	column := utf8.RuneCountInString(source[lineStart:offset]) + 1
	return Position{Line: lineIndex + 1, Column: column}
}
