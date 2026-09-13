package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
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
	grammar *sitter.Language
	rules   structureRules
}

func newJavaScriptLanguage() languageAdapter {
	return &javaScriptLanguage{
		grammar: sitter.NewLanguage(javascript.Language()),
		rules: structureRules{
			structuralTypes:       javaScriptStructural,
			contextTypes:          javaScriptContext,
			containerTypes:        newStringSet("class_declaration", "export_statement"),
			classDeclarationTypes: newStringSet("class_declaration"),
			classBodyTypes:        newStringSet("class_body"),
			exportTypes:           newStringSet("export_statement"),
			functionLikeTypes:     newStringSet("function_declaration", "generator_function_declaration", "method_definition", "arrow_function", "function"),
			blockTypes:            newStringSet("statement_block"),
			jsxElementTypes:       newStringSet("jsx_element", "jsx_self_closing_element", "jsx_fragment"),
			nameFieldCandidates:   newStringSet("identifier", "property_identifier", "field_identifier", "private_property_identifier"),
		},
	}
}

func (*javaScriptLanguage) ID() string                         { return "javascript" }
func (language *javaScriptLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *javaScriptLanguage) Rules() *structureRules    { return &language.rules }
func (language *javaScriptLanguage) Navigation() navigationAdapter {
	return ecmaNavigationAdapter(&language.rules)
}
func (language *javaScriptLanguage) Outline(root *sitter.Node, content string) []Symbol {
	return outlineTSJS(root, content, &language.rules)
}
