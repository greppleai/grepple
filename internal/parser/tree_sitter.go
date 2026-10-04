package parser

import (
	"fmt"
	"runtime"
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
	raw            *sitter.Tree
	source         string
	offset         uint
	origin         syntaxPoint
	language       string
	parent         *syntaxNode
	embedded       map[uintptr]*syntaxTree
	injectedErrors map[uintptr]bool
	diagnostics    []ParseDiagnostic
}

func (tree *syntaxTree) Close() {
	if tree != nil && tree.raw != nil {
		for _, child := range tree.embedded {
			child.Close()
		}
		tree.embedded = nil
		tree.raw.Close()
		tree.raw = nil
	}
}

func (tree *syntaxTree) RootNode() *syntaxNode {
	if tree == nil || tree.raw == nil {
		return nil
	}
	return wrapSyntaxNode(tree.raw.RootNode(), tree.source, tree)
}

type syntaxNode struct {
	raw    *sitter.Node
	source string
	tree   *syntaxTree
}

type syntaxPoint struct {
	Row    uint
	Column uint
}

func wrapSyntaxNode(node *sitter.Node, source string, owner ...*syntaxTree) *syntaxNode {
	if node == nil {
		return nil
	}
	value := &syntaxNode{raw: node, source: source}
	if len(owner) != 0 {
		value.tree = owner[0]
	}
	return value
}

func (node *syntaxNode) Kind() string    { return node.raw.Kind() }
func (node *syntaxNode) StartByte() uint { return node.raw.StartByte() + node.offset() }
func (node *syntaxNode) EndByte() uint   { return node.raw.EndByte() + node.offset() }
func (node *syntaxNode) Text() string {
	start, end := int(node.StartByte()), int(node.EndByte())
	if start < 0 || end < start || end > len(node.source) {
		return ""
	}
	return node.source[start:end]
}
func (node *syntaxNode) StartPosition() syntaxPoint {
	point := node.raw.StartPosition()
	return node.sourcePoint(point.Row, point.Column)
}
func (node *syntaxNode) EndPosition() syntaxPoint {
	point := node.raw.EndPosition()
	return node.sourcePoint(point.Row, point.Column)
}
func (node *syntaxNode) ChildCount() uint { return node.raw.ChildCount() + node.injectionCount() }
func (node *syntaxNode) NamedChildCount() uint {
	return node.raw.NamedChildCount() + node.injectionCount()
}
func (node *syntaxNode) IsNamed() bool   { return node.raw.IsNamed() }
func (node *syntaxNode) IsExtra() bool   { return node.raw.IsExtra() }
func (node *syntaxNode) IsError() bool   { return node.raw.IsError() }
func (node *syntaxNode) IsMissing() bool { return node.raw.IsMissing() }
func (node *syntaxNode) HasError() bool {
	return node.raw.HasError() || node.tree != nil && node.tree.injectedErrors[node.ID()]
}

func (node *syntaxNode) ID() uintptr { return node.raw.Id() }
func (node *syntaxNode) Child(index uint) *syntaxNode {
	if index == node.raw.ChildCount() {
		return node.injectionRoot()
	}
	return wrapSyntaxNode(node.raw.Child(index), node.source, node.tree)
}
func (node *syntaxNode) NamedChild(index uint) *syntaxNode {
	if index == node.raw.NamedChildCount() {
		return node.injectionRoot()
	}
	return wrapSyntaxNode(node.raw.NamedChild(index), node.source, node.tree)
}
func (node *syntaxNode) ChildByFieldName(name string) *syntaxNode {
	return wrapSyntaxNode(node.raw.ChildByFieldName(name), node.source, node.tree)
}
func (node *syntaxNode) Parent() *syntaxNode {
	if node.raw.Parent() == nil && node.tree != nil {
		return node.tree.parent
	}
	return wrapSyntaxNode(node.raw.Parent(), node.source, node.tree)
}
func (node *syntaxNode) FieldNameForChild(index uint32) string {
	if index >= uint32(node.raw.ChildCount()) {
		return ""
	}
	return node.raw.FieldNameForChild(index)
}
func (node *syntaxNode) FieldNameForNamedChild(index uint32) string {
	if index >= uint32(node.raw.NamedChildCount()) {
		return ""
	}
	return node.raw.FieldNameForNamedChild(index)
}
func (node *syntaxNode) PrevNamedSibling() *syntaxNode {
	return wrapSyntaxNode(node.raw.PrevNamedSibling(), node.source, node.tree)
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
	if node.injectionRoot() != nil {
		return &syntaxCursor{node: node}
	}
	return &syntaxCursor{raw: node.raw.Walk(), source: node.source, tree: node.tree}
}

type syntaxCursor struct {
	raw      *sitter.TreeCursor
	source   string
	tree     *syntaxTree
	node     *syntaxNode
	siblings []*syntaxNode
	index    int
}

func (cursor *syntaxCursor) Close() {
	if cursor.raw != nil {
		cursor.raw.Close()
	}
}
func (cursor *syntaxCursor) GotoFirstChild() bool {
	if cursor.raw != nil {
		return cursor.raw.GotoFirstChild()
	}
	children := cursor.node.Children()
	if len(children) == 0 {
		return false
	}
	cursor.siblings, cursor.index, cursor.node = children, 0, children[0]
	return true
}
func (cursor *syntaxCursor) GotoNextSibling() bool {
	if cursor.raw != nil {
		return cursor.raw.GotoNextSibling()
	}
	if cursor.index+1 >= len(cursor.siblings) {
		return false
	}
	cursor.index++
	cursor.node = cursor.siblings[cursor.index]
	return true
}
func (cursor *syntaxCursor) Node() *syntaxNode {
	if cursor.raw != nil {
		return wrapSyntaxNode(cursor.raw.Node(), cursor.source, cursor.tree)
	}
	return cursor.node
}
func (cursor *syntaxCursor) FieldName() string {
	if cursor.raw != nil {
		return cursor.raw.FieldName()
	}
	if parent := cursor.node.Parent(); parent != nil {
		return parent.FieldNameForChild(uint32(cursor.index))
	}
	return ""
}
func (node *syntaxNode) StartLine() int { return int(node.StartPosition().Row) + 1 }
func (node *syntaxNode) EndLine() int   { return int(node.EndPosition().Row) + 1 }

func (node *syntaxNode) offset() uint {
	if node.tree == nil {
		return 0
	}
	return node.tree.offset
}
func (node *syntaxNode) sourcePoint(row, column uint) syntaxPoint {
	if node.tree != nil {
		if row == 0 {
			column += node.tree.origin.Column
		}
		row += node.tree.origin.Row
	}
	return syntaxPoint{Row: row, Column: column}
}
func (node *syntaxNode) injectionRoot() *syntaxNode {
	if node.tree == nil {
		return nil
	}
	if tree := node.tree.embedded[node.ID()]; tree != nil {
		return tree.RootNode()
	}
	return nil
}
func (node *syntaxNode) injectionCount() uint {
	if node.injectionRoot() != nil {
		return 1
	}
	return 0
}

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
