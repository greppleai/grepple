package parser

import (
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type goLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newGoLanguage() languageAdapter {
	return &goLanguage{
		grammar: newSyntaxLanguage(golang.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("import_declaration", "function_declaration", "method_declaration", "type_declaration", "var_declaration", "const_declaration"),
			contextTypes:          newStringSet("function_declaration", "method_declaration", "type_declaration", "var_declaration", "const_declaration"),
			containerTypes:        newStringSet("type_declaration"),
			classDeclarationTypes: newStringSet(),
			classBodyTypes:        newStringSet(),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_declaration", "method_declaration"),
			nameFieldCandidates:   newStringSet("identifier", "field_identifier", "type_identifier"),
		},
	}
}

func (*goLanguage) ID() string                       { return "go" }
func (language *goLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *goLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *goLanguage) Rules() *structureRules { return &language.rules }
func (language *goLanguage) Navigation() navigationAdapter {
	return goNavigationAdapter(&language.rules)
}
func (language *goLanguage) Outline(root *syntaxNode, content string) []Symbol {
	return outlineGo(root, content, &language.rules)
}

func outlineGo(root *syntaxNode, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, child := range root.NamedChildren() {
		switch child.Kind() {
		case "function_declaration":
			out = append(out, symbolFrom("func", extractNodeName(child, content, config), child, content))
		case "method_declaration":
			name := goReceiver(child, content) + extractNodeName(child, content, config)
			out = append(out, symbolFrom("method", name, child, content))
		case "type_declaration":
			out = append(out, goTypes(child, content, config)...)
		case "const_declaration":
			out = append(out, goValues(child, content, "const")...)
		case "var_declaration":
			out = append(out, goValues(child, content, "var")...)
		}
	}
	return out
}

// goReceiver renders a method's receiver type as a "(*Type)." prefix so methods
// read as members of their type even though Go declares them at file scope.
func goReceiver(method *syntaxNode, _ string) string {
	recv := method.ChildByFieldName("receiver")
	if recv == nil {
		return ""
	}
	for _, pd := range recv.NamedChildren() {
		if t := pd.ChildByFieldName("type"); t != nil {
			return "(" + t.Text() + ")."
		}
	}
	return ""
}

func goTypes(decl *syntaxNode, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, spec := range decl.NamedChildren() {
		if spec.Kind() != "type_spec" && spec.Kind() != "type_alias" {
			continue
		}
		name := extractNodeName(spec, content, config)
		kind := "type"
		var children []Symbol
		if t := spec.ChildByFieldName("type"); t != nil {
			switch t.Kind() {
			case "struct_type":
				kind = "struct"
			case "interface_type":
				kind = "interface"
				children = goInterfaceMembers(t, content, config)
			}
		}
		sym := symbolFrom(kind, name, spec, content)
		sym.Children = children
		out = append(out, sym)
	}
	return out
}

func goInterfaceMembers(iface *syntaxNode, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, m := range iface.NamedChildren() {
		if m.Kind() == "method_elem" || m.Kind() == "method_spec" {
			out = append(out, symbolFrom("method", extractNodeName(m, content, config), m, content))
		}
	}
	return out
}

func goValues(decl *syntaxNode, content string, label string) []Symbol {
	var out []Symbol
	for _, spec := range decl.NamedChildren() {
		if spec.Kind() != "const_spec" && spec.Kind() != "var_spec" {
			continue
		}
		for _, id := range spec.NamedChildren() {
			if id.Kind() == "identifier" {
				out = append(out, symbolFrom(label, id.Text(), spec, content))
			}
		}
	}
	return out
}
