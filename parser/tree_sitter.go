package parser

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// parseInvocations counts tree-sitter parses for package tests.
var parseInvocations atomic.Int64

// syntaxLanguage, syntaxTree, and syntaxNode form the private syntax backend
// boundary. No parser engine or language adapter needs to expose Tree-sitter
// types; this file alone translates between project-owned syntax handles and
// the concrete Tree-sitter implementation.
type syntaxLanguage struct {
	raw *sitter.Language
}

func newSyntaxLanguage(pointer unsafe.Pointer) syntaxLanguage {
	return syntaxLanguage{raw: sitter.NewLanguage(pointer)}
}

func (language syntaxLanguage) valid() bool        { return language.raw != nil }
func (language syntaxLanguage) abiVersion() uint32 { return language.raw.AbiVersion() }

type syntaxTree struct {
	raw    *sitter.Tree
	source string
}

func (tree *syntaxTree) Close() {
	if tree != nil && tree.raw != nil {
		tree.raw.Close()
		tree.raw = nil
	}
}

func (tree *syntaxTree) RootNode() *syntaxNode {
	if tree == nil || tree.raw == nil {
		return nil
	}
	return wrapSyntaxNode(tree.raw.RootNode(), tree.source)
}

type syntaxNode struct {
	raw    *sitter.Node
	source string
}

type syntaxPoint struct {
	Row    uint
	Column uint
}

func wrapSyntaxNode(node *sitter.Node, source string) *syntaxNode {
	if node == nil {
		return nil
	}
	return &syntaxNode{raw: node, source: source}
}

func (node *syntaxNode) Kind() string    { return node.raw.Kind() }
func (node *syntaxNode) StartByte() uint { return node.raw.StartByte() }
func (node *syntaxNode) EndByte() uint   { return node.raw.EndByte() }
func (node *syntaxNode) Text() string {
	start, end := int(node.StartByte()), int(node.EndByte())
	if start < 0 || end < start || end > len(node.source) {
		return ""
	}
	return node.source[start:end]
}
func (node *syntaxNode) StartPosition() syntaxPoint {
	point := node.raw.StartPosition()
	return syntaxPoint{Row: point.Row, Column: point.Column}
}
func (node *syntaxNode) EndPosition() syntaxPoint {
	point := node.raw.EndPosition()
	return syntaxPoint{Row: point.Row, Column: point.Column}
}
func (node *syntaxNode) ChildCount() uint      { return node.raw.ChildCount() }
func (node *syntaxNode) NamedChildCount() uint { return node.raw.NamedChildCount() }
func (node *syntaxNode) IsNamed() bool         { return node.raw.IsNamed() }
func (node *syntaxNode) IsExtra() bool         { return node.raw.IsExtra() }
func (node *syntaxNode) IsError() bool         { return node.raw.IsError() }
func (node *syntaxNode) IsMissing() bool       { return node.raw.IsMissing() }
func (node *syntaxNode) HasError() bool        { return node.raw.HasError() }
func (node *syntaxNode) ID() uintptr           { return node.raw.Id() }
func (node *syntaxNode) Child(index uint) *syntaxNode {
	return wrapSyntaxNode(node.raw.Child(index), node.source)
}
func (node *syntaxNode) NamedChild(index uint) *syntaxNode {
	return wrapSyntaxNode(node.raw.NamedChild(index), node.source)
}
func (node *syntaxNode) ChildByFieldName(name string) *syntaxNode {
	return wrapSyntaxNode(node.raw.ChildByFieldName(name), node.source)
}
func (node *syntaxNode) Parent() *syntaxNode {
	return wrapSyntaxNode(node.raw.Parent(), node.source)
}
func (node *syntaxNode) FieldNameForChild(index uint32) string {
	return node.raw.FieldNameForChild(index)
}
func (node *syntaxNode) FieldNameForNamedChild(index uint32) string {
	return node.raw.FieldNameForNamedChild(index)
}
func (node *syntaxNode) PrevNamedSibling() *syntaxNode {
	return wrapSyntaxNode(node.raw.PrevNamedSibling(), node.source)
}

func (node *syntaxNode) Children() []*syntaxNode {
	children := make([]*syntaxNode, 0, node.ChildCount())
	for index := uint(0); index < node.ChildCount(); index++ {
		if child := node.Child(index); child != nil {
			children = append(children, child)
		}
	}
	return children
}

func (node *syntaxNode) NamedChildren() []*syntaxNode {
	children := make([]*syntaxNode, 0, node.NamedChildCount())
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if child := node.NamedChild(index); child != nil {
			children = append(children, child)
		}
	}
	return children
}

func (node *syntaxNode) WalkNamed(visit func(*syntaxNode)) {
	if node == nil || visit == nil {
		return
	}
	visit(node)
	for _, child := range node.NamedChildren() {
		child.WalkNamed(visit)
	}
}

func (node *syntaxNode) Walk() *syntaxCursor {
	return &syntaxCursor{raw: node.raw.Walk(), source: node.source}
}

type syntaxCursor struct {
	raw    *sitter.TreeCursor
	source string
}

func (cursor *syntaxCursor) Close()               { cursor.raw.Close() }
func (cursor *syntaxCursor) GotoFirstChild() bool { return cursor.raw.GotoFirstChild() }
func (cursor *syntaxCursor) GotoNextSibling() bool {
	return cursor.raw.GotoNextSibling()
}
func (cursor *syntaxCursor) Node() *syntaxNode {
	return wrapSyntaxNode(cursor.raw.Node(), cursor.source)
}
func (cursor *syntaxCursor) FieldName() string { return cursor.raw.FieldName() }

func (node *syntaxNode) StartLine() int { return int(node.raw.StartPosition().Row) + 1 }
func (node *syntaxNode) EndLine() int   { return int(node.raw.EndPosition().Row) + 1 }

// pooledParser is a reusable Tree-sitter parser. The parser is owned by one
// goroutine between Get and Put; the returned syntax tree is independent.
type pooledParser struct {
	parser   *sitter.Parser
	language *sitter.Language
}

var parserPool = sync.Pool{
	New: func() any {
		pooled := &pooledParser{parser: sitter.NewParser()}
		runtime.SetFinalizer(pooled, func(value *pooledParser) { value.parser.Close() })
		return pooled
	},
}

func parseSyntaxTree(grammar syntaxLanguage, content string) (*syntaxTree, error) {
	parseInvocations.Add(1)
	if !grammar.valid() {
		return nil, fmt.Errorf("unsupported language")
	}
	pooled := parserPool.Get().(*pooledParser)
	defer parserPool.Put(pooled)
	rawGrammar := grammar.raw
	if pooled.language != rawGrammar {
		if err := pooled.parser.SetLanguage(rawGrammar); err != nil {
			pooled.language = nil
			return nil, err
		}
		pooled.language = rawGrammar
	}
	tree := pooled.parser.Parse([]byte(content), nil)
	if tree == nil {
		return nil, fmt.Errorf("tree-sitter returned no tree")
	}
	return &syntaxTree{raw: tree, source: content}, nil
}

// matchLines carries the matched line numbers both as a set (membership) and as
// an ascending slice (range queries). Building the sorted slice once lets
// hitsRange use binary search instead of scanning the whole map for every node
// visited during segment building (rangeHit used to be O(matches) per node).
type matchLines struct {
	set    map[int]bool
	sorted []int
}

func newMatchLines(hits map[int]bool) matchLines {
	sorted := make([]int, 0, len(hits))
	for line := range hits {
		sorted = append(sorted, line)
	}
	sort.Ints(sorted)
	return matchLines{set: hits, sorted: sorted}
}

// hitsRange reports whether any matched line falls within [start, end].
func (m matchLines) hitsRange(start, end int) bool {
	i := sort.Search(len(m.sorted), func(i int) bool { return m.sorted[i] >= start })
	return i < len(m.sorted) && m.sorted[i] <= end
}

func extractNodeName(node *syntaxNode, _ string, config *structureRules) string {
	if named := node.ChildByFieldName("name"); named != nil {
		return named.Text()
	}
	for _, child := range node.Children() {
		if config.nameFieldCandidates.contains(child.Kind()) {
			return child.Text()
		}
	}
	return ""
}

func isPunctuation(node *syntaxNode) bool {
	kind := node.Kind()
	return len(kind) == 1 && !strings.ContainsAny(kind, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}
