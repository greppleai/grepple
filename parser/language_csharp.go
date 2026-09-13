package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	csharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
)

type cSharpLanguage struct {
	grammar *sitter.Language
	rules   structureRules
}

func newCSharpLanguage() languageAdapter {
	types := newStringSet("class_declaration", "interface_declaration", "struct_declaration", "record_declaration", "enum_declaration")
	return &cSharpLanguage{
		grammar: sitter.NewLanguage(csharp.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("using_directive", "namespace_declaration", "file_scoped_namespace_declaration", "class_declaration", "interface_declaration", "struct_declaration", "record_declaration", "enum_declaration", "method_declaration", "constructor_declaration", "property_declaration", "field_declaration"),
			contextTypes:          newStringSet("namespace_declaration", "file_scoped_namespace_declaration", "class_declaration", "interface_declaration", "struct_declaration", "record_declaration", "enum_declaration", "method_declaration", "constructor_declaration", "property_declaration"),
			containerTypes:        newStringSet("namespace_declaration", "file_scoped_namespace_declaration", "class_declaration", "interface_declaration", "struct_declaration", "record_declaration", "enum_declaration"),
			classDeclarationTypes: types,
			classBodyTypes:        newStringSet("declaration_list", "enum_member_declaration_list"),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("method_declaration", "constructor_declaration", "local_function_statement", "lambda_expression", "anonymous_method_expression"),
			blockTypes:            newStringSet("block"),
			jsxElementTypes:       newStringSet(),
			nameFieldCandidates:   newStringSet("identifier"),
		},
	}
}

func (*cSharpLanguage) ID() string                         { return "csharp" }
func (language *cSharpLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *cSharpLanguage) Rules() *structureRules    { return &language.rules }
func (language *cSharpLanguage) Navigation() navigationAdapter {
	return cSharpNavigationAdapter(&language.rules)
}
func (language *cSharpLanguage) Outline(root *sitter.Node, content string) []Symbol {
	return cSharpDeclarations(namedChildren(root), content, &language.rules)
}

func cSharpDeclarations(nodes []*sitter.Node, content string, rules *structureRules) []Symbol {
	var symbols []Symbol
	for _, node := range nodes {
		kind := cSharpSymbolKind(node.Kind())
		if kind == "" {
			continue
		}
		name := extractNodeName(node, content, rules)
		if name == "" {
			name = descendantName(node, content, rules.nameFieldCandidates)
		}
		symbol := symbolFrom(kind, name, node, content)
		if kind == "namespace" || rules.classDeclarationTypes.contains(node.Kind()) {
			if body := cSharpBody(node); body != nil {
				symbol.Children = cSharpDeclarations(namedChildren(body), content, rules)
			}
		}
		symbols = append(symbols, symbol)
	}
	return symbols
}

func cSharpBody(node *sitter.Node) *sitter.Node {
	if body := node.ChildByFieldName("body"); body != nil {
		return body
	}
	for _, child := range namedChildren(node) {
		if child.Kind() == "declaration_list" || child.Kind() == "enum_member_declaration_list" {
			return child
		}
	}
	return nil
}

func cSharpSymbolKind(kind string) string {
	switch kind {
	case "namespace_declaration", "file_scoped_namespace_declaration":
		return "namespace"
	case "class_declaration":
		return "class"
	case "interface_declaration":
		return "interface"
	case "struct_declaration":
		return "struct"
	case "record_declaration":
		return "record"
	case "enum_declaration":
		return "enum"
	case "method_declaration":
		return "method"
	case "constructor_declaration":
		return "constructor"
	case "property_declaration":
		return "property"
	case "field_declaration":
		return "field"
	}
	return ""
}
