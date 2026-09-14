package parser

func outlineTSJS(root *syntaxNode, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, child := range tsTopLevel(root) {
		out = append(out, tsSymbolsFor(child, content, config)...)
	}
	return out
}

// tsTopLevel unwraps `export`/`export default` statements so the inner
// declaration is treated as a top-level definition.
func tsTopLevel(root *syntaxNode) []*syntaxNode {
	var out []*syntaxNode
	for _, child := range root.NamedChildren() {
		if child.Kind() == "export_statement" {
			for _, inner := range child.NamedChildren() {
				if tsIsDecl(inner.Kind()) {
					out = append(out, inner)
				}
			}
			continue
		}
		out = append(out, child)
	}
	return out
}

func tsIsDecl(kind string) bool {
	switch kind {
	case "class_declaration", "abstract_class_declaration", "interface_declaration",
		"function_declaration", "generator_function_declaration", "type_alias_declaration",
		"enum_declaration", "lexical_declaration", "variable_declaration":
		return true
	}
	return false
}

func tsSymbolsFor(node *syntaxNode, content string, config *structureRules) []Symbol {
	name := extractNodeName(node, content, config)
	switch node.Kind() {
	case "class_declaration", "abstract_class_declaration":
		sym := symbolFrom("class", name, node, content)
		sym.Children = tsBodyMembers(node, content, config)
		return []Symbol{sym}
	case "interface_declaration":
		sym := symbolFrom("interface", name, node, content)
		sym.Children = tsBodyMembers(node, content, config)
		return []Symbol{sym}
	case "enum_declaration":
		return []Symbol{symbolFrom("enum", name, node, content)}
	case "function_declaration", "generator_function_declaration":
		return []Symbol{symbolFrom("function", name, node, content)}
	case "type_alias_declaration":
		return []Symbol{symbolFrom("type", name, node, content)}
	case "lexical_declaration", "variable_declaration":
		return tsVariables(node, content, config)
	}
	return nil
}

func tsVariables(decl *syntaxNode, content string, config *structureRules) []Symbol {
	var out []Symbol
	for _, d := range decl.NamedChildren() {
		if d.Kind() != "variable_declarator" {
			continue
		}
		name := ""
		if n := d.ChildByFieldName("name"); n != nil {
			name = n.Text()
		}
		kind := "const"
		if v := d.ChildByFieldName("value"); v != nil && config.functionLikeTypes.contains(v.Kind()) {
			kind = "function"
		}
		out = append(out, symbolFrom(kind, name, d, content))
	}
	return out
}

// tsBodyMembers lists the members of a class or interface body.
func tsBodyMembers(node *syntaxNode, content string, config *structureRules) []Symbol {
	body := node.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	var out []Symbol
	for _, m := range body.NamedChildren() {
		switch m.Kind() {
		case "method_definition", "method_signature":
			out = append(out, symbolFrom("method", extractNodeName(m, content, config), m, content))
		case "public_field_definition", "field_definition", "property_signature":
			out = append(out, symbolFrom("property", extractNodeName(m, content, config), m, content))
		}
	}
	return out
}
