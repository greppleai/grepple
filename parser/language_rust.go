package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

type rustLanguage struct {
	grammar *sitter.Language
	rules   structureRules
}

func newRustLanguage() languageAdapter {
	return &rustLanguage{
		grammar: sitter.NewLanguage(rust.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("use_declaration", "function_item", "function_signature_item", "struct_item", "enum_item", "trait_item", "impl_item", "mod_item", "type_item", "const_item", "static_item", "macro_definition"),
			contextTypes:          newStringSet("function_item", "function_signature_item", "struct_item", "enum_item", "trait_item", "impl_item", "mod_item", "macro_definition"),
			containerTypes:        newStringSet("trait_item", "impl_item", "mod_item"),
			classDeclarationTypes: newStringSet("trait_item", "impl_item"),
			classBodyTypes:        newStringSet("declaration_list", "field_declaration_list", "enum_variant_list"),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_item", "function_signature_item", "closure_expression"),
			blockTypes:            newStringSet("block"),
			jsxElementTypes:       newStringSet(),
			nameFieldCandidates:   newStringSet("identifier", "type_identifier", "field_identifier"),
		},
	}
}

func (*rustLanguage) ID() string                         { return "rust" }
func (language *rustLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *rustLanguage) Rules() *structureRules    { return &language.rules }
func (language *rustLanguage) Outline(root *sitter.Node, content string) []Symbol {
	return rustDeclarations(namedChildren(root), content, &language.rules)
}

func rustDeclarations(nodes []*sitter.Node, content string, rules *structureRules) []Symbol {
	var symbols []Symbol
	for _, node := range nodes {
		kind := rustSymbolKind(node.Kind())
		if kind == "" {
			continue
		}
		name := extractNodeName(node, content, rules)
		if node.Kind() == "impl_item" {
			if target := node.ChildByFieldName("type"); target != nil {
				name = nodeText(target, content)
			}
		}
		symbol := symbolFrom(kind, name, node, content)
		if rules.containerTypes.contains(node.Kind()) {
			if body := rustBody(node); body != nil {
				symbol.Children = rustDeclarations(namedChildren(body), content, rules)
			}
		}
		symbols = append(symbols, symbol)
	}
	return symbols
}

func rustBody(node *sitter.Node) *sitter.Node {
	if body := node.ChildByFieldName("body"); body != nil {
		return body
	}
	for _, child := range namedChildren(node) {
		if child.Kind() == "declaration_list" {
			return child
		}
	}
	return nil
}

func rustSymbolKind(kind string) string {
	switch kind {
	case "function_item":
		return "function"
	case "function_signature_item":
		return "method"
	case "struct_item":
		return "struct"
	case "enum_item":
		return "enum"
	case "trait_item":
		return "trait"
	case "impl_item":
		return "impl"
	case "mod_item":
		return "module"
	case "type_item":
		return "type"
	case "const_item":
		return "const"
	case "static_item":
		return "static"
	case "macro_definition":
		return "macro"
	}
	return ""
}
