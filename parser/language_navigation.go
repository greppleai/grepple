package parser

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func goNavigationMethodName(node *sitter.Node, content string, rules *structureRules) string {
	name := extractNodeName(node, content, rules)
	receiver := strings.TrimSuffix(strings.TrimPrefix(goReceiver(node, content), "("), ").")
	receiver = strings.TrimPrefix(receiver, "*")
	if receiver == "" {
		return name
	}
	return receiver + "." + name
}

func goNavigationAdapter(rules *structureRules) navigationAdapter {
	adapter := &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression"), extraContainerTypes: newStringSet("type_spec"),
		fieldContainerTypes: newStringSet("type_spec"), isCallable: func(node *sitter.Node) bool {
			if rules.functionLikeTypes.contains(node.Kind()) || node.Kind() == "method_elem" || node.Kind() == "method_spec" {
				return true
			}
			callableType := node.ChildByFieldName("type")
			return node.Kind() == "field_declaration" && callableType != nil && callableType.Kind() == "function_type"
		},
		requiresContainer: func(node *sitter.Node) bool {
			return node.Kind() == "method_elem" || node.Kind() == "method_spec" || node.Kind() == "field_declaration"
		},
		declarationName: func(node *sitter.Node, content string, _ *navigationEnvelope) string {
			if node.Kind() == "method_declaration" {
				return goNavigationMethodName(node, content, rules)
			}
			return extractNodeName(node, content, rules)
		},
		declarationKind: func(node *sitter.Node, container string) string {
			switch node.Kind() {
			case "field_declaration":
				return "field"
			case "function_declaration":
				return "func"
			}
			return defaultNavigationDeclarationKind(node, container)
		},
		visibility: func(_ *sitter.Node, name, _ string) NavigationVisibility {
			return goNavigationVisibility(navigationTerminal(name))
		},
		sourceFacts: goNavigationSourceFacts,
		returnCallableName: func(node *sitter.Node, container, content string, _ *navigationAdapterConfig) string {
			if node.Kind() == "method_declaration" {
				return goNavigationMethodName(node, content, rules)
			}
			name := extractNodeName(node, content, rules)
			if container != "" && name != "" && !strings.Contains(name, ".") {
				return container + "." + name
			}
			return name
		},
		callableReturnBinding: func(node *sitter.Node, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "result")
		},
	}
	return adapter
}

func ecmaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression", "new_expression"), extraContainerTypes: newStringSet("interface_declaration"),
		fieldContainerTypes: newStringSet("class_declaration", "interface_declaration"), selfBindingName: "this", selfBindingFromContainer: true,
		visibility: typeScriptAdapterVisibility, sourceFacts: typeScriptNavigationSourceFacts,
		callableReturnBinding: func(node *sitter.Node, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "return_type", "type")
		},
	}
}

func pythonNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call"), visibility: func(_ *sitter.Node, name, _ string) NavigationVisibility {
		return pythonNavigationVisibility(navigationTerminal(name))
	}}
}

func javaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("method_invocation", "object_creation_expression", "explicit_constructor_invocation"), visibility: func(node *sitter.Node, _ string, content string) NavigationVisibility {
		return javaNavigationVisibility(node, navigationDeclarationHeader(node, content))
	}}
}

func kotlinNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), visibility: func(node *sitter.Node, _ string, content string) NavigationVisibility {
		return visibilityFromModifiers(navigationDeclarationHeader(node, content), true)
	}}
}

func cSharpNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("invocation_expression", "object_creation_expression"), visibility: func(node *sitter.Node, _ string, content string) NavigationVisibility {
		return cSharpNavigationVisibility(navigationDeclarationHeader(node, content))
	}}
}

func cFamilyNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), declarationName: func(node *sitter.Node, content string, _ *navigationEnvelope) string {
		return cFamilyName(node, content, rules)
	}}
}

func rustNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), containerName: func(node *sitter.Node, content string, envelope *navigationEnvelope) string {
		if node.Kind() == "impl_item" {
			if target := node.ChildByFieldName("type"); target != nil {
				return nodeText(target, content)
			}
		}
		return defaultNavigationDeclarationName(node, content, rules, envelope)
	}, visibility: func(node *sitter.Node, _ string, content string) NavigationVisibility {
		return visibilityFromRequiredModifier(navigationDeclarationHeader(node, content), "pub")
	}}
}

func shellNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("command")}
}

func defaultNavigationDeclarationName(node *sitter.Node, content string, rules *structureRules, envelope *navigationEnvelope) string {
	if envelope != nil && envelope.assignedName != "" {
		return envelope.assignedName
	}
	return extractNodeName(node, content, rules)
}

func defaultNavigationDeclarationKind(node *sitter.Node, container string) string {
	kind := node.Kind()
	if strings.Contains(kind, "constructor") {
		return "constructor"
	}
	if strings.Contains(kind, "method") || container != "" {
		return "method"
	}
	return "function"
}

func navigationTerminal(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func goNavigationSourceFacts(root *sitter.Node, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	walkNodes(root, func(node *sitter.Node) {
		switch node.Kind() {
		case "package_clause":
			packageName = navigationPackageName(node, content)
		case "import_spec":
			addGoNavigationImport(imports, node, content)
		}
	})
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, packageName, fields
}

func typeScriptNavigationSourceFacts(root *sitter.Node, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	walkNodes(root, func(node *sitter.Node) {
		if node.Kind() == "import_statement" {
			addTypeScriptNavigationImports(imports, node, content)
		}
	})
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, packageName, fields
}

func collectNavigationSourceFields(root *sitter.Node, content string, imports map[string]navigationImport, fields map[string]map[string]navigationBinding, adapter navigationAdapter) {
	walkNodes(root, func(node *sitter.Node) {
		addNavigationContainerFields(fields, node, content, imports, adapter)
	})
}

func typeScriptAdapterVisibility(node *sitter.Node, name, content string) NavigationVisibility {
	return typeScriptNavigationVisibility(node, navigationTerminal(name), navigationDeclarationHeader(node, content))
}
