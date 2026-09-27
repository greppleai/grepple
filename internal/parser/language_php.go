package parser

import php "github.com/tree-sitter/tree-sitter-php/bindings/go"

type phpLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newPHPLanguage() languageAdapter {
	return &phpLanguage{
		grammar: newSyntaxLanguage(php.LanguagePHP()),
		rules: structureRules{
			structuralTypes:       newStringSet("namespace_definition", "namespace_use_declaration", "function_definition", "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration", "method_declaration", "property_declaration", "const_declaration"),
			contextTypes:          newStringSet("namespace_definition", "function_definition", "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration", "method_declaration"),
			containerTypes:        newStringSet("namespace_definition", "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration"),
			classDeclarationTypes: newStringSet("class_declaration", "interface_declaration", "trait_declaration", "enum_declaration"),
			classBodyTypes:        newStringSet("declaration_list", "enum_declaration_list"),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_definition", "method_declaration", "anonymous_function_creation_expression", "arrow_function"),
			nameFieldCandidates:   newStringSet("name", "namespace_name"),
		},
	}
}

func (*phpLanguage) ID() string                       { return "php" }
func (language *phpLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *phpLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *phpLanguage) Rules() *structureRules { return &language.rules }
func (language *phpLanguage) Navigation() navigationAdapter {
	return phpNavigationAdapter(&language.rules)
}
func (language *phpLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return phpDeclarations(root.NamedChildren(), content)
}

func phpDeclarations(nodes []*syntaxNode, content string) []Symbol {
	var symbols []Symbol
	for _, node := range nodes {
		kind := phpSymbolKind(node.Kind())
		if kind == "" {
			// Unbracketed namespaces contain sibling declarations; PHP's
			// top-level expression statements are not declarations.
			continue
		}
		name := ""
		if named := node.ChildByFieldName("name"); named != nil {
			name = named.Text()
		}
		symbol := symbolFrom(kind, name, node, content)
		if body := node.ChildByFieldName("body"); body != nil && (kind == "namespace" || kind == "class" || kind == "interface" || kind == "trait" || kind == "enum") {
			symbol.Children = phpDeclarations(body.NamedChildren(), content)
		}
		symbols = append(symbols, symbol)
	}
	return symbols
}

func phpSymbolKind(kind string) string {
	switch kind {
	case "namespace_definition":
		return "namespace"
	case "function_definition":
		return "function"
	case "class_declaration":
		return "class"
	case "interface_declaration":
		return "interface"
	case "trait_declaration":
		return "trait"
	case "enum_declaration":
		return "enum"
	case "method_declaration":
		return "method"
	case "property_declaration":
		return "property"
	case "const_declaration":
		return "const"
	}
	return ""
}
