package parser

import (
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
)

var (
	javaScriptStructural = newStringSet(
		"import_statement", "function_declaration", "class_declaration", "method_definition",
		"lexical_declaration", "variable_declaration", "export_statement",
	)
	javaScriptContext = newStringSet(
		"function_declaration", "function", "arrow_function", "method_definition",
		"class_declaration", "lexical_declaration", "variable_declaration", "export_statement",
	)
)

type javaScriptLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newJavaScriptLanguage() languageAdapter {
	return &javaScriptLanguage{
		grammar: newSyntaxLanguage(javascript.Language()),
		rules: structureRules{
			structuralTypes:       javaScriptStructural,
			contextTypes:          javaScriptContext,
			containerTypes:        newStringSet("class_declaration", "export_statement"),
			classDeclarationTypes: newStringSet("class_declaration"),
			classBodyTypes:        newStringSet("class_body"),
			exportTypes:           newStringSet("export_statement"),
			functionLikeTypes:     newStringSet("function_declaration", "generator_function_declaration", "method_definition", "arrow_function", "function"),
			nameFieldCandidates:   newStringSet("identifier", "property_identifier", "field_identifier", "private_property_identifier"),
		},
	}
}

func (*javaScriptLanguage) ID() string                       { return "javascript" }
func (language *javaScriptLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *javaScriptLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *javaScriptLanguage) Rules() *structureRules { return &language.rules }
func (language *javaScriptLanguage) Navigation() navigationAdapter {
	return ecmaNavigationAdapter(&language.rules, false, true)
}
func (language *javaScriptLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return outlineTSJS(root, content, &language.rules)
}

func (*javaScriptLanguage) ScopeKind(nodeType string) string {
	switch nodeType {
	case "if_statement":
		return "if"
	case "while_statement":
		return "while"
	case "for_statement", "for_in_statement", "for_of_statement":
		return "for"
	case "switch_statement":
		return "switch"
	case "try_statement":
		return "try"
	case "catch_clause":
		return "catch"
	case "case_statement", "switch_case":
		return "case"
	case "function_declaration", "generator_function_declaration", "arrow_function", "function":
		return "func"
	case "method_definition":
		return "method"
	case "class_declaration", "class":
		return "class"
	}
	return ""
}
