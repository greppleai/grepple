package parser

import (
	"strings"

	kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
)

type kotlinLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newKotlinLanguage() languageAdapter {
	return &kotlinLanguage{
		grammar: newSyntaxLanguage(kotlin.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("import_header", "class_declaration", "object_declaration", "function_declaration", "property_declaration"),
			contextTypes:          newStringSet("class_declaration", "object_declaration", "function_declaration", "property_declaration"),
			containerTypes:        newStringSet("class_declaration", "object_declaration"),
			classDeclarationTypes: newStringSet("class_declaration", "object_declaration"),
			classBodyTypes:        newStringSet("class_body"),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_declaration"),
			nameFieldCandidates:   newStringSet("simple_identifier", "identifier", "type_identifier"),
		},
	}
}

func (*kotlinLanguage) ID() string                       { return "kotlin" }
func (language *kotlinLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *kotlinLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *kotlinLanguage) Rules() *structureRules { return &language.rules }
func (language *kotlinLanguage) Navigation() navigationAdapter {
	return kotlinNavigationAdapter(&language.rules)
}
func (language *kotlinLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return outlineKotlin(root, content, &language.rules)
}

func outlineKotlin(root *syntaxNode, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, child := range root.NamedChildren() {
		switch child.Kind() {
		case "class_declaration":
			kind := "class"
			if sig := declSignature(child, content); strings.Contains(sig, "interface ") {
				kind = "interface"
			}
			sym := symbolFrom(kind, extractNodeName(child, content, config), child, content)
			sym.Children = kotlinMembers(child, content, config)
			out = append(out, sym)
		case "object_declaration":
			sym := symbolFrom("object", extractNodeName(child, content, config), child, content)
			sym.Children = kotlinMembers(child, content, config)
			out = append(out, sym)
		case "function_declaration":
			out = append(out, symbolFrom("fun", extractNodeName(child, content, config), child, content))
		case "property_declaration":
			out = append(out, symbolFrom("val", kotlinPropertyName(child, content, config), child, content))
		}
	}
	return out
}

func kotlinMembers(node *syntaxNode, content string, config *structureRules) []Symbol {
	var body *syntaxNode
	for _, child := range node.NamedChildren() {
		if child.Kind() == "class_body" || child.Kind() == "enum_class_body" {
			body = child
			break
		}
	}
	if body == nil {
		return nil
	}
	var out []Symbol
	for _, m := range body.NamedChildren() {
		switch m.Kind() {
		case "function_declaration":
			out = append(out, symbolFrom("fun", extractNodeName(m, content, config), m, content))
		case "property_declaration":
			out = append(out, symbolFrom("val", kotlinPropertyName(m, content, config), m, content))
		}
	}
	return out
}

// kotlinPropertyName digs the variable name out of a property_declaration, whose
// name sits inside a variable_declaration child rather than a "name" field.
func kotlinPropertyName(node *syntaxNode, content string, config *structureRules) string {
	if name := extractNodeName(node, content, config); name != "" {
		return name
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() == "variable_declaration" {
			return extractNodeName(child, content, config)
		}
	}
	return ""
}
