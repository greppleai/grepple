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

func (*cppLanguage) ScopeKind(nodeType string) string {
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
	case "class_specifier":
		return "class"
	}
	return ""
}

func (language *cppLanguage) ScopeClassification(node ViewNode) ScopeClassification {
	kind := language.ScopeKind(node.Kind())
	if node.Kind() == "function_definition" && scopeMemberAncestor(node, language.ScopeKind) {
		kind = "method"
	}
	return standardScopeClassification(kind)
}

func (*cppLanguage) ScopeSelectorKind(role string) string {
	return functionSelectorKind(role, "function", false)
}
