package parser

import (
	"path"
	"strconv"
	"strings"
)

func goNavigationMethodName(node *syntaxNode, content string, rules *structureRules) string {
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
		fieldContainerTypes: newStringSet("type_spec"), isCallable: func(node *syntaxNode) bool {
			if rules.functionLikeTypes.contains(node.Kind()) || node.Kind() == "method_elem" || node.Kind() == "method_spec" {
				return true
			}
			callableType := node.ChildByFieldName("type")
			return node.Kind() == "field_declaration" && callableType != nil && callableType.Kind() == "function_type"
		},
		requiresContainer: func(node *syntaxNode) bool {
			return node.Kind() == "method_elem" || node.Kind() == "method_spec" || node.Kind() == "field_declaration"
		},
		declarationName: func(node *syntaxNode, content string, _ *navigationEnvelope) string {
			if node.Kind() == "method_declaration" {
				return goNavigationMethodName(node, content, rules)
			}
			return extractNodeName(node, content, rules)
		},
		declarationKind: func(node *syntaxNode, container string) string {
			switch node.Kind() {
			case "field_declaration":
				return "field"
			case "function_declaration":
				return "func"
			}
			return defaultNavigationDeclarationKind(node, container)
		},
		visibility: func(_ *syntaxNode, name, _ string) NavigationVisibility {
			return goNavigationVisibility(navigationTerminal(name))
		},
		sourceFacts: goNavigationSourceFacts, entrypoint: goNavigationEntrypoint,
		returnCallableName: func(node *syntaxNode, container, content string, _ *navigationAdapterConfig) string {
			if node.Kind() == "method_declaration" {
				return goNavigationMethodName(node, content, rules)
			}
			name := extractNodeName(node, content, rules)
			if container != "" && name != "" && !strings.Contains(name, ".") {
				return container + "." + name
			}
			return name
		},
		fieldNames: goNavigationFieldNames,
		callableReturnBinding: func(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "result")
		},
	}
	return adapter
}

func goNavigationEntrypoint(context navigationEntrypointContext) string {
	if context.node.Kind() == "function_declaration" && context.name == "main" && context.container == "" && context.packageName == "main" {
		return "process"
	}
	return ""
}

func goNavigationFieldNames(node, typeNode *syntaxNode, content string) []navigationFieldName {
	names := defaultNavigationParameterNames(node, typeNode, content)
	result := make([]navigationFieldName, 0, max(1, len(names)))
	for _, name := range names {
		result = append(result, navigationFieldName{name: name})
	}
	if len(result) == 0 && node.Kind() == "field_declaration" {
		if name := navigationTypeName(typeNode.Text()); name != "" {
			result = append(result, navigationFieldName{name: name, embedded: true})
		}
	}
	return result
}

func goNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	root.WalkNamed(func(node *syntaxNode) {
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

func addGoNavigationImport(imports map[string]navigationImport, node *syntaxNode, _ string) {
	pathNode := node.ChildByFieldName("path")
	if pathNode == nil {
		return
	}
	importPath := unquoteNavigationPath(pathNode.Text())
	if importPath == "" {
		return
	}
	alias := path.Base(importPath)
	if name := node.ChildByFieldName("name"); name != nil {
		alias = name.Text()
	}
	if alias == "" {
		return
	}
	key := alias
	if alias == "_" || alias == "." {
		key += "\x00" + strconv.Itoa(node.StartLine())
	}
	imports[key] = navigationImport{alias: alias, path: importPath, imported: "*", line: node.StartLine()}
}
