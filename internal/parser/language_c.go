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

func (*cLanguage) ScopeKind(nodeType string) string {
	switch nodeType {
	case "if_statement":
		return "if"
	case "while_statement":
		return "while"
	case "for_statement", "for_in_statement":
		return "for"
	case "switch_statement":
		return "switch"
	case "try_statement":
		return "try"
	case "catch_clause":
		return "catch"
	case "case_statement":
		return "case"
	case "function_definition", "lambda_expression":
		return "func"
	case "struct_specifier", "union_specifier":
		return "struct"
	case "enum_specifier":
		return "enum"
	}
	return ""
}

func (language *cLanguage) ScopeClassification(node ViewNode) ScopeClassification {
	kind := language.ScopeKind(node.Kind())
	if node.Kind() == "function_definition" && scopeMemberAncestor(node, language.ScopeKind) {
		kind = "method"
	}
	return standardScopeClassification(kind)
}

func (*cLanguage) ScopeSelectorKind(role string) string {
	return functionSelectorKind(role, "function", false)
}
