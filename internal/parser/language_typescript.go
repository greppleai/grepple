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
	return ecmaNavigationAdapter(&language.rules, true, true)
}
func (language *typeScriptLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return outlineTSJS(root, content, &language.rules)
}

func (*typeScriptLanguage) ScopeKind(nodeType string) string {
	switch nodeType {
	case "interface_declaration":
		return "interface"
	case "enum_declaration":
		return "enum"
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
