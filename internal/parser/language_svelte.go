package parser

import (
	"strings"

	svelte "github.com/tree-sitter-grammars/tree-sitter-svelte/bindings/go"
)

// The Svelte grammar owns markup and template blocks. Script/style contents and
// template expressions are raw text, not JavaScript, TypeScript or CSS syntax.
type svelteLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newSvelteLanguage() languageAdapter {
	structures := newStringSet("element", "self_closing_tag", "script_element", "style_element",
		"if_statement", "each_statement", "await_statement", "key_statement", "snippet_statement",
		"else_if_block", "else_block", "then_block", "catch_block",
		"html_tag", "const_tag", "debug_tag", "render_tag", "expression")
	return &svelteLanguage{
		grammar: newSyntaxLanguage(svelte.Language()),
		rules: structureRules{
			structuralTypes: structures,
			contextTypes:    structures,
			containerTypes:  newStringSet("document", "element", "if_statement", "each_statement", "await_statement", "key_statement", "snippet_statement", "else_if_block", "else_block", "then_block", "catch_block"),
		},
	}
}

func (*svelteLanguage) ID() string                       { return "svelte" }
func (language *svelteLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *svelteLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *svelteLanguage) Rules() *structureRules { return &language.rules }
func (language *svelteLanguage) Navigation() navigationAdapter {
	return &navigationAdapterConfig{
		rules:      &language.rules,
		isCallable: func(node *syntaxNode) bool { return node.Kind() == "snippet_statement" },
		declarationName: func(node *syntaxNode, _ string, _ *navigationEnvelope) string {
			return svelteChildText(svelteNamedChild(node, "snippet_start"), "snippet_name")
		},
		declarationKind: func(_ *syntaxNode, _ string) string { return "snippet" },
	}
}
func (*svelteLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return svelteOutlineChildren(root, content)
}

func svelteOutlineChildren(root *syntaxNode, content string) []Symbol {
	if root == nil {
		return nil
	}
	var symbols []Symbol
	for _, node := range root.NamedChildren() {
		kind, name := svelteOutlineName(node)
		if name == "" {
			continue
		}
		symbol := symbolFrom(kind, name, node, content)
		symbol.Children = svelteOutlineChildren(node, content)
		symbols = append(symbols, symbol)
	}
	return symbols
}

func svelteOutlineName(node *syntaxNode) (string, string) {
	switch node.Kind() {
	case "script_element":
		return "script", "script"
	case "style_element":
		return "style", "style"
	case "element":
		return "element", svelteElementName(node)
	case "self_closing_tag":
		if parent := node.Parent(); parent != nil && parent.Kind() == "element" {
			return "", ""
		}
		return "element", svelteChildText(node, "tag_name")
	case "snippet_statement":
		return "snippet", svelteChildText(svelteNamedChild(node, "snippet_start"), "snippet_name")
	case "if_statement", "each_statement", "await_statement", "key_statement":
		return "block", strings.TrimSuffix(node.Kind(), "_statement")
	case "else_if_block", "else_block", "then_block", "catch_block":
		return "block", strings.ReplaceAll(strings.TrimSuffix(node.Kind(), "_block"), "_", " ")
	default:
		return "", ""
	}
}

func svelteNamedChild(node *syntaxNode, kind string) *syntaxNode {
	if node != nil {
		for _, child := range node.NamedChildren() {
			if child.Kind() == kind {
				return child
			}
		}
	}
	return nil
}

func svelteChildText(node *syntaxNode, kind string) string {
	if child := svelteNamedChild(node, kind); child != nil {
		return child.Text()
	}
	return ""
}

func svelteElementName(node *syntaxNode) string {
	tag := svelteNamedChild(node, "start_tag")
	if tag == nil {
		tag = svelteNamedChild(node, "self_closing_tag")
	}
	return svelteChildText(tag, "tag_name")
}
