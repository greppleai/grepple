package parser

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// parseInvocations counts tree-sitter parses for package tests.
var parseInvocations atomic.Int64

type stringSet map[string]struct{}

func newStringSet(values ...string) stringSet {
	set := make(stringSet, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func (set stringSet) contains(value string) bool {
	_, ok := set[value]
	return ok
}

type languageConfig struct {
	id                    string
	grammar               *sitter.Language
	structuralTypes       stringSet
	contextTypes          stringSet
	containerTypes        stringSet
	classDeclarationTypes stringSet
	classBodyTypes        stringSet
	exportTypes           stringSet
	functionLikeTypes     stringSet
	blockTypes            stringSet
	jsxElementTypes       stringSet
	nameFieldCandidates   stringSet
}

var (
	typeScriptStructural = newStringSet(
		"import_statement", "function_declaration", "class_declaration", "method_definition",
		"interface_declaration", "type_alias_declaration", "lexical_declaration",
		"variable_declaration", "export_statement",
	)
	typeScriptContext = newStringSet(
		"function_declaration", "function", "arrow_function", "method_definition",
		"class_declaration", "interface_declaration", "type_alias_declaration",
		"lexical_declaration", "variable_declaration", "export_statement",
	)
	javaScriptStructural = newStringSet(
		"import_statement", "function_declaration", "class_declaration", "method_definition",
		"lexical_declaration", "variable_declaration", "export_statement",
	)
	javaScriptContext = newStringSet(
		"function_declaration", "function", "arrow_function", "method_definition",
		"class_declaration", "lexical_declaration", "variable_declaration", "export_statement",
	)
)

var languageConfigs = map[string]*languageConfig{
	"typescript": newTypeScriptConfig("typescript", sitter.NewLanguage(typescript.LanguageTypescript())),
	"tsx":        newTypeScriptConfig("tsx", sitter.NewLanguage(typescript.LanguageTSX())),
	"javascript": {
		id:                    "javascript",
		grammar:               sitter.NewLanguage(javascript.Language()),
		structuralTypes:       javaScriptStructural,
		contextTypes:          javaScriptContext,
		containerTypes:        newStringSet("class_declaration", "export_statement"),
		classDeclarationTypes: newStringSet("class_declaration"),
		classBodyTypes:        newStringSet("class_body"),
		exportTypes:           newStringSet("export_statement"),
		functionLikeTypes:     newStringSet("function_declaration", "method_definition", "arrow_function", "function"),
		blockTypes:            newStringSet("statement_block"),
		jsxElementTypes:       newStringSet("jsx_element", "jsx_self_closing_element", "jsx_fragment"),
		nameFieldCandidates:   newStringSet("identifier", "property_identifier", "field_identifier", "private_property_identifier"),
	},
	"go": {
		id:                    "go",
		grammar:               sitter.NewLanguage(golang.Language()),
		structuralTypes:       newStringSet("import_declaration", "function_declaration", "method_declaration", "type_declaration", "var_declaration", "const_declaration"),
		contextTypes:          newStringSet("function_declaration", "method_declaration", "type_declaration", "var_declaration", "const_declaration"),
		containerTypes:        newStringSet("type_declaration"),
		classDeclarationTypes: newStringSet(),
		classBodyTypes:        newStringSet(),
		exportTypes:           newStringSet(),
		functionLikeTypes:     newStringSet("function_declaration", "method_declaration"),
		blockTypes:            newStringSet("block"),
		jsxElementTypes:       newStringSet(),
		nameFieldCandidates:   newStringSet("identifier", "field_identifier", "type_identifier"),
	},
	"java": {
		id:                    "java",
		grammar:               sitter.NewLanguage(java.Language()),
		structuralTypes:       newStringSet("import_declaration", "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "method_declaration", "constructor_declaration", "field_declaration"),
		contextTypes:          newStringSet("class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "method_declaration", "constructor_declaration", "field_declaration"),
		containerTypes:        newStringSet("class_declaration", "interface_declaration", "enum_declaration", "record_declaration"),
		classDeclarationTypes: newStringSet("class_declaration", "interface_declaration", "enum_declaration", "record_declaration"),
		classBodyTypes:        newStringSet("class_body", "interface_body", "enum_body", "record_body"),
		exportTypes:           newStringSet(),
		functionLikeTypes:     newStringSet("method_declaration", "constructor_declaration"),
		blockTypes:            newStringSet("block"),
		jsxElementTypes:       newStringSet(),
		nameFieldCandidates:   newStringSet("identifier", "type_identifier"),
	},
	"kotlin": {
		id:                    "kotlin",
		grammar:               sitter.NewLanguage(kotlin.Language()),
		structuralTypes:       newStringSet("import_header", "class_declaration", "object_declaration", "function_declaration", "property_declaration"),
		contextTypes:          newStringSet("class_declaration", "object_declaration", "function_declaration", "property_declaration"),
		containerTypes:        newStringSet("class_declaration", "object_declaration"),
		classDeclarationTypes: newStringSet("class_declaration", "object_declaration"),
		classBodyTypes:        newStringSet("class_body"),
		exportTypes:           newStringSet(),
		functionLikeTypes:     newStringSet("function_declaration"),
		blockTypes:            newStringSet("function_body", "control_structure_body", "statements"),
		jsxElementTypes:       newStringSet(),
		nameFieldCandidates:   newStringSet("simple_identifier", "identifier", "type_identifier"),
	},
}

func newTypeScriptConfig(id string, grammar *sitter.Language) *languageConfig {
	return &languageConfig{
		id:                    id,
		grammar:               grammar,
		structuralTypes:       typeScriptStructural,
		contextTypes:          typeScriptContext,
		containerTypes:        newStringSet("class_declaration", "export_statement"),
		classDeclarationTypes: newStringSet("class_declaration"),
		classBodyTypes:        newStringSet("class_body"),
		exportTypes:           newStringSet("export_statement"),
		functionLikeTypes:     newStringSet("function_declaration", "method_definition", "arrow_function", "function"),
		blockTypes:            newStringSet("statement_block"),
		jsxElementTypes:       newStringSet("jsx_element", "jsx_self_closing_element", "jsx_fragment"),
		nameFieldCandidates:   newStringSet("identifier", "type_identifier", "property_identifier", "field_identifier", "private_property_identifier"),
	}
}

func configForLanguage(id string) *languageConfig {
	return languageConfigs[id]
}

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

func parseTree(config *languageConfig, content string) (*sitter.Tree, error) {
	parseInvocations.Add(1)
	if config == nil || config.grammar == nil {
		return nil, fmt.Errorf("unsupported language")
	}
	parser := parserPool.Get().(*pooledParser)
	defer parserPool.Put(parser)
	if parser.lang != config.grammar {
		if err := parser.p.SetLanguage(config.grammar); err != nil {
			parser.lang = nil
			return nil, err
		}
		parser.lang = config.grammar
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

func extractNodeName(node *sitter.Node, content string, config *languageConfig) string {
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
