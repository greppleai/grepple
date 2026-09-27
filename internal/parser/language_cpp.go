package parser

import (
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

type cppLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newCPPLanguage() languageAdapter {
	return &cppLanguage{
		grammar: newSyntaxLanguage(cpp.Language()),
		rules:   newCFamilyRules(true),
	}
}

func (*cppLanguage) ID() string                       { return "cpp" }
func (language *cppLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *cppLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *cppLanguage) Rules() *structureRules { return &language.rules }
func (language *cppLanguage) Navigation() navigationAdapter {
	return cFamilyNavigationAdapter(&language.rules)
}
func (language *cppLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return cFamilyDeclarations(root.NamedChildren(), content, &language.rules, true, false)
}
