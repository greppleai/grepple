package parser

import swift "github.com/alex-pinkus/tree-sitter-swift/bindings/go"

type swiftLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newSwiftLanguage() languageAdapter {
	return &swiftLanguage{
		grammar: newSyntaxLanguage(swift.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("import_declaration", "class_declaration", "protocol_declaration", "function_declaration", "protocol_function_declaration", "init_declaration", "property_declaration", "protocol_property_declaration", "typealias_declaration"),
			contextTypes:          newStringSet("class_declaration", "protocol_declaration", "function_declaration", "protocol_function_declaration", "init_declaration", "property_declaration"),
			containerTypes:        newStringSet("class_declaration", "protocol_declaration"),
			classDeclarationTypes: newStringSet("class_declaration", "protocol_declaration"),
			classBodyTypes:        newStringSet("class_body", "enum_class_body", "protocol_body"),
			functionLikeTypes:     newStringSet("function_declaration", "protocol_function_declaration", "init_declaration"),
			nameFieldCandidates:   newStringSet("simple_identifier", "type_identifier"),
		},
	}
}

func (*swiftLanguage) ID() string                       { return "swift" }
func (language *swiftLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *swiftLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *swiftLanguage) Rules() *structureRules { return &language.rules }
func (language *swiftLanguage) Navigation() navigationAdapter {
	return swiftNavigationAdapter(&language.rules)
}
func (language *swiftLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return swiftOutline(root.NamedChildren(), content)
}

func swiftOutline(nodes []*syntaxNode, content string) []Symbol {
	var symbols []Symbol
	for _, node := range nodes {
		symbols = append(symbols, swiftOutlineNode(node, content)...)
	}
	return symbols
}

func swiftOutlineNode(node *syntaxNode, content string) []Symbol {
	switch node.Kind() {
	case "class_declaration", "protocol_declaration":
		name := swiftDeclarationName(node)
		if name == "" {
			return nil
		}
		symbol := symbolFrom(swiftTypeKind(node), name, node, content)
		if body := node.ChildByFieldName("body"); body != nil {
			symbol.Children = swiftOutline(body.NamedChildren(), content)
		}
		return []Symbol{symbol}
	case "function_declaration", "protocol_function_declaration", "init_declaration":
		return swiftOutlineCallable(node, content)
	case "property_declaration", "protocol_property_declaration":
		if name := swiftDeclarationName(node); name != "" {
			return []Symbol{symbolFrom("field", name, node, content)}
		}
	case "typealias_declaration":
		if name := swiftDeclarationName(node); name != "" {
			return []Symbol{symbolFrom("type", name, node, content)}
		}
	case "enum_entry":
		var entries []Symbol
		for _, child := range node.NamedChildren() {
			if child.Kind() == "simple_identifier" {
				entries = append(entries, symbolFrom("const", child.Text(), child, content))
			}
		}
		return entries
	case "protocol_body":
		// The grammar can wrap protocol requirements in another body node.
		return swiftOutline(node.NamedChildren(), content)
	}
	return nil
}

func swiftOutlineCallable(node *syntaxNode, content string) []Symbol {
	name := swiftDeclarationName(node)
	if name == "" {
		return nil
	}
	kind := "function"
	if node.Kind() == "init_declaration" {
		kind = "constructor"
	} else if parent := node.Parent(); parent != nil && parent.Kind() != "source_file" {
		kind = "method"
	}
	return []Symbol{symbolFrom(kind, name, node, content)}
}
func swiftTypeKind(node *syntaxNode) string {
	if node.Kind() == "protocol_declaration" {
		return "interface"
	}
	if declaration := node.ChildByFieldName("declaration_kind"); declaration != nil {
		switch declaration.Text() {
		case "struct", "enum", "extension":
			return declaration.Text()
		}
	}
	return "class"
}

func swiftDeclarationName(node *syntaxNode) string {
	if node.Kind() == "init_declaration" {
		return "init"
	}
	name := node.ChildByFieldName("name")
	if name == nil {
		return ""
	}
	if name.Kind() == "pattern" {
		if bound := name.ChildByFieldName("bound_identifier"); bound != nil {
			return bound.Text()
		}
	}
	return name.Text()
}
