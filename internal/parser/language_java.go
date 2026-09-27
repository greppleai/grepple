package parser

import (
	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

type javaLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newJavaLanguage() languageAdapter {
	return &javaLanguage{
		grammar: newSyntaxLanguage(java.Language()),
		rules: structureRules{
			structuralTypes:         newStringSet("import_declaration", "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "method_declaration", "constructor_declaration", "field_declaration"),
			contextTypes:            newStringSet("class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "method_declaration", "constructor_declaration", "field_declaration"),
			containerTypes:          newStringSet("class_declaration", "interface_declaration", "enum_declaration", "record_declaration"),
			classDeclarationTypes:   newStringSet("class_declaration", "interface_declaration", "enum_declaration", "record_declaration"),
			classBodyTypes:          newStringSet("class_body", "interface_body", "enum_body", "record_body"),
			exportTypes:             newStringSet(),
			functionLikeTypes:       newStringSet("method_declaration", "constructor_declaration"),
			functionExpressionTypes: newStringSet("lambda_expression"),
			nameFieldCandidates:     newStringSet("identifier", "type_identifier"),
		},
	}
}

func (*javaLanguage) ID() string                       { return "java" }
func (language *javaLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *javaLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *javaLanguage) Rules() *structureRules { return &language.rules }
func (language *javaLanguage) Navigation() navigationAdapter {
	return javaNavigationAdapter(&language.rules)
}
func (language *javaLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return outlineJava(root, content, &language.rules)
}

func outlineJava(root *syntaxNode, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, child := range root.NamedChildren() {
		if config.classDeclarationTypes.contains(child.Kind()) {
			out = append(out, javaType(child, content, config))
		}
	}
	return out
}

func javaType(node *syntaxNode, content string, config *structureRules) Symbol {
	sym := symbolFrom(javaTypeKind(node.Kind()), extractNodeName(node, content, config), node, content)
	sym.Children = javaMembers(node, content, config)
	return sym
}

func javaTypeKind(kind string) string {
	switch kind {
	case "interface_declaration":
		return "interface"
	case "enum_declaration":
		return "enum"
	case "record_declaration":
		return "record"
	default:
		return "class"
	}
}

func javaMembers(node *syntaxNode, content string, config *structureRules) []Symbol {
	body := node.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	var out []Symbol
	for _, m := range body.NamedChildren() {
		switch m.Kind() {
		case "method_declaration":
			out = append(out, symbolFrom("method", extractNodeName(m, content, config), m, content))
		case "constructor_declaration":
			out = append(out, symbolFrom("constructor", extractNodeName(m, content, config), m, content))
		case "field_declaration":
			for _, d := range m.NamedChildren() {
				if d.Kind() == "variable_declarator" {
					out = append(out, symbolFrom("field", extractNodeName(d, content, config), m, content))
				}
			}
		case "enum_constant":
			out = append(out, symbolFrom("const", extractNodeName(m, content, config), m, content))
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
			out = append(out, javaType(m, content, config))
		}
	}
	return out
}
