package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	c "github.com/tree-sitter/tree-sitter-c/bindings/go"
)

type cLanguage struct {
	grammar *sitter.Language
	rules   structureRules
}

func newCLanguage() languageAdapter {
	return &cLanguage{
		grammar: sitter.NewLanguage(c.Language()),
		rules:   newCFamilyRules(false),
	}
}

func (*cLanguage) ID() string                         { return "c" }
func (language *cLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *cLanguage) Rules() *structureRules    { return &language.rules }
func (language *cLanguage) Outline(root *sitter.Node, content string) []Symbol {
	return cFamilyDeclarations(namedChildren(root), content, &language.rules, false, false)
}
