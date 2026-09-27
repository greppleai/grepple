package parser

import dart "github.com/nielsenko/tree-sitter-dart/bindings/go"

type dartLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newDartLanguage() languageAdapter {
	return &dartLanguage{
		grammar: newSyntaxLanguage(dart.Language()),
		rules: structureRules{
			structuralTypes:         newStringSet("import_or_export", "part_directive", "class_declaration", "enum_declaration", "mixin_declaration", "extension_declaration", "extension_type_declaration", "function_declaration", "top_level_variable_declaration", "type_alias", "class_member"),
			contextTypes:            newStringSet("class_declaration", "enum_declaration", "mixin_declaration", "extension_declaration", "extension_type_declaration", "function_declaration", "method_declaration", "class_member"),
			containerTypes:          newStringSet("class_declaration", "enum_declaration", "mixin_declaration", "extension_declaration", "extension_type_declaration"),
			classDeclarationTypes:   newStringSet("class_declaration", "enum_declaration", "mixin_declaration", "extension_declaration", "extension_type_declaration"),
			classBodyTypes:          newStringSet("class_body", "enum_body", "mixin_body", "extension_body", "extension_type_body"),
			functionLikeTypes:       newStringSet("function_declaration", "method_declaration", "getter_declaration", "setter_declaration", "external_function_declaration", "constructor_signature"),
			functionExpressionTypes: newStringSet("function_expression"),
			nameFieldCandidates:     newStringSet("identifier", "type_identifier"),
		},
	}
}

func (*dartLanguage) ID() string                       { return "dart" }
func (language *dartLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *dartLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *dartLanguage) Rules() *structureRules { return &language.rules }
func (language *dartLanguage) Navigation() navigationAdapter {
	return dartNavigationAdapter(&language.rules)
}
func (language *dartLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return dartOutline(root.NamedChildren(), content, &language.rules)
}

func dartOutline(nodes []*syntaxNode, content string, rules *structureRules) []Symbol {
	var symbols []Symbol
	for _, node := range nodes {
		switch node.Kind() {
		case "class_declaration", "enum_declaration", "mixin_declaration", "extension_declaration", "extension_type_declaration":
			kind := dartOutlineTypeKind(node.Kind())
			symbol := symbolFrom(kind, extractNodeName(node, content, rules), node, content)
			if body := node.ChildByFieldName("body"); body != nil {
				symbol.Children = dartOutline(body.NamedChildren(), content, rules)
			}
			symbols = append(symbols, symbol)
		case "class_member", "declaration":
			symbols = append(symbols, dartOutline(node.NamedChildren(), content, rules)...)
		case "function_declaration", "method_declaration", "constructor_signature", "getter_declaration", "setter_declaration", "external_function_declaration":
			kind := "function"
			if node.Kind() == "method_declaration" {
				kind = "method"
			} else if node.Kind() == "constructor_signature" {
				kind = "constructor"
			}
			symbols = append(symbols, symbolFrom(kind, dartCallableName(node), node, content))
		case "top_level_variable_declaration", "initialized_identifier_list", "static_final_declaration_list":
			symbols = append(symbols, dartOutline(node.NamedChildren(), content, rules)...)
		case "initialized_identifier":
			symbols = append(symbols, symbolFrom("field", extractNodeName(node, content, rules), node, content))
		case "type_alias":
			symbols = append(symbols, symbolFrom("type", extractNodeName(node, content, rules), node, content))
		case "enum_constant":
			symbols = append(symbols, symbolFrom("const", extractNodeName(node, content, rules), node, content))
		}
	}
	return symbols
}

func dartOutlineTypeKind(kind string) string {
	switch kind {
	case "enum_declaration":
		return "enum"
	case "mixin_declaration":
		return "mixin"
	case "extension_declaration", "extension_type_declaration":
		return "extension"
	default:
		return "class"
	}
}
func dartCallableName(node *syntaxNode) string {
	if name := node.ChildByFieldName("name"); name != nil {
		return name.Text()
	}
	if signature := dartSignature(node); signature != nil {
		if name := signature.ChildByFieldName("name"); name != nil {
			return name.Text()
		}
	}
	return ""
}

func dartSignature(node *syntaxNode) *syntaxNode {
	signature := node.ChildByFieldName("signature")
	if signature != nil && signature.Kind() == "method_signature" {
		for _, child := range signature.NamedChildren() {
			switch child.Kind() {
			case "function_signature", "getter_signature", "setter_signature", "constructor_signature", "factory_constructor_signature", "operator_signature":
				return child
			}
		}
	}
	return signature
}
