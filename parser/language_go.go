package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type goLanguage struct {
	grammar *sitter.Language
	rules   structureRules
}

func newGoLanguage() languageAdapter {
	return &goLanguage{
		grammar: sitter.NewLanguage(golang.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("import_declaration", "function_declaration", "method_declaration", "type_declaration", "var_declaration", "const_declaration"),
			contextTypes:          newStringSet("function_declaration", "method_declaration", "type_declaration", "var_declaration", "const_declaration"),
			containerTypes:        newStringSet("type_declaration"),
			classDeclarationTypes: newStringSet(),
			classBodyTypes:        newStringSet(),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_declaration", "method_declaration"),
			blockTypes:            newStringSet("block"),
			jsxElementTypes:       newStringSet(),
			nameFieldCandidates:   newStringSet("identifier", "field_identifier", "type_identifier"),
		},
	}
}

func (*goLanguage) ID() string                         { return "go" }
func (language *goLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *goLanguage) Rules() *structureRules    { return &language.rules }
func (language *goLanguage) Outline(root *sitter.Node, content string) []Symbol {
	return outlineGo(root, content, &language.rules)
}

func outlineGo(root *sitter.Node, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, child := range namedChildren(root) {
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
func goReceiver(method *sitter.Node, content string) string {
	recv := method.ChildByFieldName("receiver")
	if recv == nil {
		return ""
	}
	for _, pd := range namedChildren(recv) {
		if t := pd.ChildByFieldName("type"); t != nil {
			return "(" + nodeText(t, content) + ")."
		}
	}
	return ""
}

func goTypes(decl *sitter.Node, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, spec := range namedChildren(decl) {
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

func goInterfaceMembers(iface *sitter.Node, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, m := range namedChildren(iface) {
		if m.Kind() == "method_elem" || m.Kind() == "method_spec" {
			out = append(out, symbolFrom("method", extractNodeName(m, content, config), m, content))
		}
	}
	return out
}

func goValues(decl *sitter.Node, content string, label string) []Symbol {
	var out []Symbol
	for _, spec := range namedChildren(decl) {
		if spec.Kind() != "const_spec" && spec.Kind() != "var_spec" {
			continue
		}
		for _, id := range namedChildren(spec) {
			if id.Kind() == "identifier" {
				out = append(out, symbolFrom(label, nodeText(id, content), spec, content))
			}
		}
	}
	return out
}
