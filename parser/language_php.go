package parser

import (
	php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	"strings"
)

type phpLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newPHPLanguage() languageAdapter {
	return &phpLanguage{
		grammar: newSyntaxLanguage(php.LanguagePHP()),
		rules: structureRules{
			structuralTypes:       newStringSet("namespace_definition", "namespace_use_declaration", "function_definition", "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration", "method_declaration", "property_declaration", "const_declaration"),
			contextTypes:          newStringSet("namespace_definition", "function_definition", "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration", "method_declaration", "property_declaration"),
			containerTypes:        newStringSet("namespace_definition", "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration"),
			classDeclarationTypes: newStringSet("class_declaration", "interface_declaration", "trait_declaration", "enum_declaration"),
			classBodyTypes:        newStringSet("declaration_list", "enum_declaration_list"),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_definition", "method_declaration", "anonymous_function_creation_expression", "arrow_function"),
			nameFieldCandidates:   newStringSet("name", "identifier", "variable_name"),
		},
	}
}

func (*phpLanguage) ID() string                { return "php" }
func (l *phpLanguage) Grammar() syntaxLanguage { return l.grammar }
func (l *phpLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(l.grammar, content)
}
func (l *phpLanguage) Rules() *structureRules        { return &l.rules }
func (l *phpLanguage) Navigation() navigationAdapter { return phpNavigationAdapter(&l.rules) }
func (l *phpLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return phpDeclarations(root.NamedChildren(), content, &l.rules)
}

func phpDeclarations(nodes []*syntaxNode, content string, rules *structureRules) []Symbol {
	var out []Symbol
	for _, node := range nodes {
		if node.Kind() == "php_tag" || node.Kind() == "text" {
			continue
		}
		kind := phpSymbolKind(node.Kind())
		if kind == "" {
			continue
		}
		if kind == "property" {
			for _, child := range node.NamedChildren() {
				if child.Kind() != "property_element" {
					continue
				}
				for _, item := range child.NamedChildren() {
					if item.Kind() == "variable_name" {
						out = append(out, symbolFrom(kind, strings.TrimPrefix(item.Text(), "$"), node, content))
						break
					}
				}
			}
			continue
		}
		name := extractNodeName(node, content, rules)
		if name == "" && node.Kind() == "method_declaration" {
			name = descendantName(node, content, rules.nameFieldCandidates)
		}
		if name == "" && kind != "namespace" {
			continue
		}
		sym := symbolFrom(kind, name, node, content)
		if rules.containerTypes.contains(node.Kind()) {
			if body := node.ChildByFieldName("body"); body != nil {
				sym.Children = phpDeclarations(body.NamedChildren(), content, rules)
			}
		}
		out = append(out, sym)
	}
	return out
}

func phpSymbolKind(kind string) string {
	switch kind {
	case "namespace_definition":
		return "namespace"
	case "function_definition":
		return "function"
	case "method_declaration":
		return "method"
	case "class_declaration":
		return "class"
	case "interface_declaration":
		return "interface"
	case "trait_declaration":
		return "trait"
	case "enum_declaration":
		return "enum"
	case "property_declaration":
		return "property"
	case "const_declaration":
		return "const"
	}
	return ""
}
