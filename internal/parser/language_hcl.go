package parser

import (
	"strconv"
	"strings"

	hcl "github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go"
)

type hclLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newHCLLanguage() languageAdapter {
	return &hclLanguage{
		grammar: newSyntaxLanguage(hcl.Language()),
		rules: structureRules{
			structuralTypes: newStringSet("block", "attribute"),
			contextTypes:    newStringSet("body", "block", "attribute"),
			containerTypes:  newStringSet("body", "block"),
			classBodyTypes:  newStringSet("body"),
		},
	}
}

func (*hclLanguage) ID() string                       { return "hcl" }
func (language *hclLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *hclLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *hclLanguage) Rules() *structureRules { return &language.rules }
func (language *hclLanguage) Navigation() navigationAdapter {
	return hclNavigationAdapter(&language.rules)
}
func (*hclLanguage) Outline(root *syntaxNode, content string) []Symbol {
	if root == nil {
		return nil
	}
	for _, child := range root.NamedChildren() {
		if child.Kind() == "body" {
			return hclOutlineBody(child, content)
		}
	}
	return nil
}

func hclOutlineBody(body *syntaxNode, content string) []Symbol {
	var symbols []Symbol
	for _, node := range body.NamedChildren() {
		if symbol, ok := hclOutlineNode(node, content); ok {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

func hclOutlineNode(node *syntaxNode, content string) (Symbol, bool) {
	switch node.Kind() {
	case "block":
		name := hclBlockName(node)
		if name == "" {
			return Symbol{}, false
		}
		symbol := symbolFrom("block", name, node, content)
		for _, child := range node.NamedChildren() {
			if child.Kind() == "body" {
				symbol.Children = hclOutlineBody(child, content)
				break
			}
		}
		return symbol, true
	case "attribute":
		for _, child := range node.NamedChildren() {
			if child.Kind() == "identifier" {
				return symbolFrom("attribute", child.Text(), node, content), true
			}
		}
	}
	return Symbol{}, false
}

// Block labels can be quoted literals or bare identifiers. Dynamic templates
// have no stable address, so do not invent a name for them.
func hclBlockName(block *syntaxNode) string {
	var labels []string
	for _, child := range block.NamedChildren() {
		switch child.Kind() {
		case "identifier":
			labels = append(labels, child.Text())
		case "string_lit":
			text := child.Text()
			if strings.Contains(text, "${") || strings.Contains(text, "%{") {
				return ""
			}
			label, err := strconv.Unquote(text)
			if err != nil || label == "" {
				return ""
			}
			labels = append(labels, label)
		case "block_start":
			return strings.Join(labels, ".")
		}
	}
	return ""
}
