package parser

import (
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

var (
	typeScriptStructural = newStringSet(
		"import_statement", "function_declaration", "class_declaration", "method_definition",
		"interface_declaration", "type_alias_declaration", "lexical_declaration",
		"variable_declaration", "export_statement",
	)
	typeScriptContext = newStringSet(
		"function_declaration", "function", "arrow_function", "method_definition",
		"class_declaration", "interface_declaration", "type_alias_declaration",
		"lexical_declaration", "variable_declaration", "export_statement",
	)
)

type typeScriptLanguage struct {
	id      string
	grammar syntaxLanguage
	rules   structureRules
}

func newTypeScriptLanguage(id string, tsx bool) languageAdapter {
	grammar := newSyntaxLanguage(typescript.LanguageTypescript())
	if tsx {
		grammar = newSyntaxLanguage(typescript.LanguageTSX())
	}
	return &typeScriptLanguage{
		id:      id,
		grammar: grammar,
		rules: structureRules{
			structuralTypes:       typeScriptStructural,
			contextTypes:          typeScriptContext,
			containerTypes:        newStringSet("class_declaration", "export_statement"),
			classDeclarationTypes: newStringSet("class_declaration"),
			classBodyTypes:        newStringSet("class_body"),
			exportTypes:           newStringSet("export_statement"),
			functionLikeTypes:     newStringSet("function_declaration", "generator_function_declaration", "method_definition", "method_signature", "arrow_function", "function"),
			blockTypes:            newStringSet("statement_block"),
			jsxElementTypes:       newStringSet("jsx_element", "jsx_self_closing_element", "jsx_fragment"),
			nameFieldCandidates:   newStringSet("identifier", "type_identifier", "property_identifier", "field_identifier", "private_property_identifier"),
		},
	}
}

func (language *typeScriptLanguage) ID() string              { return language.id }
func (language *typeScriptLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *typeScriptLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *typeScriptLanguage) Rules() *structureRules { return &language.rules }
func (language *typeScriptLanguage) Navigation() navigationAdapter {
	return ecmaNavigationAdapter(&language.rules)
}
func (language *typeScriptLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return outlineTSJS(root, content, &language.rules)
}
