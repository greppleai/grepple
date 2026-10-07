package parser

import (
	"strings"

	css "github.com/tree-sitter/tree-sitter-css/bindings/go"
)

type cssLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newCSSLanguage() languageAdapter {
	structures := newStringSet("rule_set", "declaration", "media_statement", "supports_statement", "scope_statement", "keyframes_statement", "keyframe_block", "at_rule", "import_statement", "charset_statement", "namespace_statement")
	return &cssLanguage{
		grammar: newSyntaxLanguage(css.Language()),
		rules: structureRules{
			structuralTypes: structures, contextTypes: structures,
			containerTypes: newStringSet("stylesheet", "block", "keyframe_block_list"),
		},
	}
}

func (*cssLanguage) ID() string                       { return "css" }
func (language *cssLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *cssLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *cssLanguage) Rules() *structureRules { return &language.rules }
func (language *cssLanguage) Navigation() navigationAdapter {
	return &navigationAdapterConfig{
		rules: &language.rules,
		isCallable: func(node *syntaxNode) bool {
			return node.Kind() == "rule_set" || node.Kind() == "declaration" || node.Kind() == "keyframes_statement"
		},
		declarationName: func(node *syntaxNode, _ string, _ *navigationEnvelope) string {
			_, name := cssOutlineName(node)
			return name
		},
		declarationKind: func(node *syntaxNode, _ string) string {
			kind, _ := cssOutlineName(node)
			return kind
		},
	}
}
func (*cssLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return cssOutlineChildren(root, content)
}

func cssOutlineChildren(root *syntaxNode, content string) []Symbol {
	if root == nil {
		return nil
	}
	var symbols []Symbol
	for _, node := range root.NamedChildren() {
		kind, name := cssOutlineName(node)
		if name == "" {
			if node.Kind() == "block" || node.Kind() == "keyframe_block_list" {
				symbols = append(symbols, cssOutlineChildren(node, content)...)
			}
			continue
		}
		symbol := symbolFrom(kind, name, node, content)
		symbol.Children = cssOutlineChildren(node, content)
		symbols = append(symbols, symbol)
	}
	return symbols
}

func cssOutlineName(node *syntaxNode) (string, string) {
	switch node.Kind() {
	case "rule_set":
		return "rule", cssChildText(node, "selectors")
	case "declaration":
		return "property", cssChildText(node, "property_name")
	case "keyframes_statement":
		return "keyframes", cssChildText(node, "keyframes_name")
	case "media_statement", "supports_statement", "scope_statement", "at_rule", "keyframe_block":
		return "block", cssBlockHeader(node)
	case "import_statement", "charset_statement", "namespace_statement":
		return "directive", strings.TrimSpace(node.Text())
	default:
		return "", ""
	}
}

func cssChildText(node *syntaxNode, kind string) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == kind {
			return child.Text()
		}
	}
	return ""
}

func cssBlockHeader(node *syntaxNode) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "block" {
			return strings.TrimSpace(node.Text()[:child.StartByte()-node.StartByte()])
		}
	}
	return strings.TrimSpace(node.Text())
}
