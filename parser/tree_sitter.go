package parser

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// parseInvocations counts tree-sitter parses for package tests.
var parseInvocations atomic.Int64

// pooledParser is a reusable tree-sitter parser. Allocating a parser is a cgo
// call that mallocs a C TSParser, so we recycle them across files instead of
// building and freeing one per parse. lang caches the currently-assigned grammar
// so we skip the redundant SetLanguage when consecutive parses share a language.
type pooledParser struct {
	p    *sitter.Parser
	lang *sitter.Language
}

// parserPool recycles tree-sitter parsers across the concurrent search workers.
// A parser taken with Get is owned exclusively by that goroutine until Put, so
// there is no concurrent use of a single (non-thread-safe) parser. The binding
// sets no finalizer on Parser, and sync.Pool may drop entries during GC, so we
// attach one here to free the underlying C parser if a pooled entry is reclaimed
// instead of being reused.
var parserPool = sync.Pool{
	New: func() any {
		pp := &pooledParser{p: sitter.NewParser()}
		runtime.SetFinalizer(pp, func(x *pooledParser) { x.p.Close() })
		return pp
	},
}

func parseTree(adapter languageAdapter, content string) (*sitter.Tree, error) {
	parseInvocations.Add(1)
	if adapter == nil || adapter.Grammar() == nil {
		return nil, fmt.Errorf("unsupported language")
	}
	parser := parserPool.Get().(*pooledParser)
	defer parserPool.Put(parser)
	grammar := adapter.Grammar()
	if parser.lang != grammar {
		if err := parser.p.SetLanguage(grammar); err != nil {
			parser.lang = nil
			return nil, err
		}
		parser.lang = grammar
	}
	// The returned tree is independent of the parser, so the parser can be
	// reused (and returned to the pool) immediately after Parse.
	tree := parser.p.Parse([]byte(content), nil)
	if tree == nil {
		return nil, fmt.Errorf("tree-sitter returned no tree")
	}
	return tree, nil
}

func nodeStart(node *sitter.Node) int { return int(node.StartPosition().Row) + 1 }
func nodeEnd(node *sitter.Node) int   { return int(node.EndPosition().Row) + 1 }

func namedChildren(node *sitter.Node) []*sitter.Node {
	children := make([]*sitter.Node, 0, node.NamedChildCount())
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if child := node.NamedChild(index); child != nil {
			children = append(children, child)
		}
	}
	return children
}

func allChildren(node *sitter.Node) []*sitter.Node {
	children := make([]*sitter.Node, 0, node.ChildCount())
	for index := uint(0); index < node.ChildCount(); index++ {
		if child := node.Child(index); child != nil {
			children = append(children, child)
		}
	}
	return children
}

func walkNodes(node *sitter.Node, visit func(*sitter.Node)) {
	visit(node)
	for _, child := range namedChildren(node) {
		walkNodes(child, visit)
	}
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

func nodeText(node *sitter.Node, content string) string {
	start, end := int(node.StartByte()), int(node.EndByte())
	if start < 0 || end < start || end > len(content) {
		return ""
	}
	return content[start:end]
}

func extractNodeName(node *sitter.Node, content string, config *structureRules) string {
	if named := node.ChildByFieldName("name"); named != nil {
		return nodeText(named, content)
	}
	for _, child := range allChildren(node) {
		if config.nameFieldCandidates.contains(child.Kind()) {
			return nodeText(child, content)
		}
	}
	return ""
}

func isPunctuation(node *sitter.Node) bool {
	kind := node.Kind()
	return len(kind) == 1 && !strings.ContainsAny(kind, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}
