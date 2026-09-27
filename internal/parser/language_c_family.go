package parser

func newCFamilyRules(cpp bool) structureRules {
	structural := newStringSet("preproc_include", "function_definition", "type_definition", "struct_specifier", "union_specifier", "enum_specifier", "declaration")
	context := newStringSet("function_definition", "type_definition", "struct_specifier", "union_specifier", "enum_specifier")
	containers := newStringSet("struct_specifier", "union_specifier", "enum_specifier")
	classes := newStringSet("struct_specifier", "union_specifier")
	if cpp {
		for _, kind := range []string{"namespace_definition", "class_specifier", "template_declaration", "concept_definition"} {
			structural[kind] = struct{}{}
			context[kind] = struct{}{}
		}
		containers["namespace_definition"] = struct{}{}
		containers["class_specifier"] = struct{}{}
		classes["class_specifier"] = struct{}{}
	}
	return structureRules{
		structuralTypes:       structural,
		contextTypes:          context,
		containerTypes:        containers,
		classDeclarationTypes: classes,
		classBodyTypes:        newStringSet("field_declaration_list", "declaration_list"),
		exportTypes:           newStringSet("template_declaration"),
		functionLikeTypes:     newStringSet("function_definition", "lambda_expression"),
		nameFieldCandidates:   newStringSet("identifier", "field_identifier", "type_identifier", "namespace_identifier"),
	}
}

func cFamilyDeclarations(nodes []*syntaxNode, content string, rules *structureRules, cpp, member bool) []Symbol {
	var symbols []Symbol
	for _, node := range nodes {
		if node.Kind() == "template_declaration" {
			symbols = append(symbols, cFamilyDeclarations(node.NamedChildren(), content, rules, cpp, member)...)
			continue
		}
		kind := cFamilySymbolKind(node.Kind(), cpp, member)
		if kind == "" {
			continue
		}
		name := cFamilyName(node, content, rules)
		symbol := symbolFrom(kind, name, node, content)
		if rules.containerTypes.contains(node.Kind()) {
			if body := cFamilyBody(node); body != nil {
				symbol.Children = cFamilyDeclarations(body.NamedChildren(), content, rules, cpp, true)
			}
		}
		symbols = append(symbols, symbol)
	}
	return symbols
}

func cFamilySymbolKind(kind string, cpp, member bool) string {
	switch kind {
	case "function_definition":
		if cpp && member {
			return "method"
		}
		return "function"
	case "type_definition":
		return "typedef"
	case "struct_specifier":
		return "struct"
	case "union_specifier":
		return "union"
	case "enum_specifier":
		return "enum"
	case "namespace_definition":
		if cpp {
			return "namespace"
		}
	case "class_specifier":
		if cpp {
			return "class"
		}
	case "concept_definition":
		if cpp {
			return "concept"
		}
	}
	return ""
}

func cFamilyName(node *syntaxNode, content string, rules *structureRules) string {
	for _, field := range []string{"name", "declarator"} {
		if child := node.ChildByFieldName(field); child != nil {
			if name := descendantName(child, content, rules.nameFieldCandidates); name != "" {
				return name
			}
		}
	}
	return descendantName(node, content, rules.nameFieldCandidates)
}

func cFamilyBody(node *syntaxNode) *syntaxNode {
	if body := node.ChildByFieldName("body"); body != nil {
		return body
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() == "field_declaration_list" || child.Kind() == "declaration_list" {
			return child
		}
	}
	return nil
}
