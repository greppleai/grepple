package parser

import (
	bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
)

type shellLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newShellLanguage() languageAdapter {
	return &shellLanguage{
		grammar: newSyntaxLanguage(bash.Language()),
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

func (*shellLanguage) ID() string                       { return "shell" }
func (language *shellLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *shellLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *shellLanguage) Rules() *structureRules { return &language.rules }
func (language *shellLanguage) Navigation() navigationAdapter {
	return shellNavigationAdapter(&language.rules)
}
func (language *shellLanguage) Outline(root *syntaxNode, content string) []Symbol {
	var symbols []Symbol
	for _, child := range root.NamedChildren() {
		if child.Kind() == "function_definition" {
			symbols = append(symbols, symbolFrom("function", extractNodeName(child, content, &language.rules), child, content))
		}
	}
	return symbols
}
