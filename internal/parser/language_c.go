package parser

import (
	c "github.com/tree-sitter/tree-sitter-c/bindings/go"
)

type cLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newCLanguage() languageAdapter {
	return &cLanguage{
		grammar: newSyntaxLanguage(c.Language()),
		rules:   newCFamilyRules(false),
	}
}

func (*cLanguage) ID() string                       { return "c" }
func (language *cLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *cLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *cLanguage) Rules() *structureRules { return &language.rules }
func (language *cLanguage) Navigation() navigationAdapter {
	return cFamilyNavigationAdapter(&language.rules)
}
func (language *cLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return cFamilyDeclarations(root.NamedChildren(), content, &language.rules, false, false)
}
