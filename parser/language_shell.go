package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
)

type shellLanguage struct {
	grammar *sitter.Language
	rules   structureRules
}

func newShellLanguage() languageAdapter {
	return &shellLanguage{
		grammar: sitter.NewLanguage(bash.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("function_definition", "variable_assignment"),
			contextTypes:          newStringSet("function_definition", "if_statement", "for_statement", "while_statement", "case_statement", "subshell"),
			containerTypes:        newStringSet("function_definition"),
			classDeclarationTypes: newStringSet(),
			classBodyTypes:        newStringSet("compound_statement", "do_group"),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_definition"),
			blockTypes:            newStringSet("compound_statement", "do_group"),
			jsxElementTypes:       newStringSet(),
			nameFieldCandidates:   newStringSet("word", "command_name"),
		},
	}
}

func (*shellLanguage) ID() string                         { return "shell" }
func (language *shellLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *shellLanguage) Rules() *structureRules    { return &language.rules }
func (language *shellLanguage) Outline(root *sitter.Node, content string) []Symbol {
	var symbols []Symbol
	for _, child := range namedChildren(root) {
		if child.Kind() == "function_definition" {
			symbols = append(symbols, symbolFrom("function", extractNodeName(child, content, &language.rules), child, content))
		}
	}
	return symbols
}
