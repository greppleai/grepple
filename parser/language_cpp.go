package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

type cppLanguage struct {
	grammar *sitter.Language
	rules   structureRules
}

func newCPPLanguage() languageAdapter {
	return &cppLanguage{
		grammar: sitter.NewLanguage(cpp.Language()),
		rules:   newCFamilyRules(true),
	}
}

func (*cppLanguage) ID() string                         { return "cpp" }
func (language *cppLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *cppLanguage) Rules() *structureRules    { return &language.rules }
func (language *cppLanguage) Outline(root *sitter.Node, content string) []Symbol {
	return cFamilyDeclarations(namedChildren(root), content, &language.rules, true, false)
}
