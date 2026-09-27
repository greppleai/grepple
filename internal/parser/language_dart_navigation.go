package parser

import "strings"

func dartNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression", "constructor_invocation"),
		wrapperTypes:          newStringSet("class_member", "declaration"),
		fieldContainerTypes:   rules.classDeclarationTypes,
		fieldDeclarationTypes: newStringSet("declaration"),
		fieldFacts:            true, memberAccessFacts: true, typeReferenceFacts: true,
		parameterTypes: newStringSet("formal_parameter"), parameterType: dartNavigationParameterType,
		selfBindingName: "this", selfBindingFromContainer: true,
		declarationName:       func(node *syntaxNode, _ string, _ *navigationEnvelope) string { return dartCallableName(node) },
		fieldNames:            dartNavigationFieldNames,
		fieldType:             dartNavigationFieldType,
		callableReturnBinding: dartNavigationReturnBinding,
		sourceFacts:           dartNavigationSourceFacts,
		visibility: func(_ *syntaxNode, name, _ string) NavigationVisibility {
			if strings.HasPrefix(navigationTerminal(name), "_") {
				return NavigationVisibilityNonPublic
			}
			return NavigationVisibilityPublic
		},
		entrypoint: dartNavigationEntrypoint,
	}
}

func dartNavigationFieldType(node *syntaxNode) *syntaxNode {
	if node.Parent() == nil || node.Parent().Kind() != "class_member" {
		return nil
	}
	return dartDirectType(node)
}

func dartNavigationParameterType(node *syntaxNode) *syntaxNode { return dartDirectType(node) }

func dartDirectType(node *syntaxNode) *syntaxNode {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "type" {
			return child
		}
	}
	return nil
}

func dartNavigationFieldNames(node, _ *syntaxNode, _ string) []navigationFieldName {
	var fields []navigationFieldName
	for _, child := range node.NamedChildren() {
		if child.Kind() != "initialized_identifier_list" && child.Kind() != "static_final_declaration_list" {
			continue
		}
		for _, identifier := range child.NamedChildren() {
			if name := identifier.ChildByFieldName("name"); name != nil {
				fields = append(fields, navigationFieldName{name: name.Text()})
			}
		}
	}
	return fields
}

func dartNavigationReturnBinding(node *syntaxNode, _ string, imports map[string]navigationImport) navigationBinding {
	if signature := dartSignature(node); signature != nil {
		return navigationReturnBindingFromFields(signature, "", imports, "return_type")
	}
	return navigationBinding{}
}

func dartNavigationEntrypoint(context navigationEntrypointContext) string {
	if context.node.Kind() != "function_declaration" || context.name != "main" || context.container != "" || context.node.Parent() == nil || context.node.Parent().Kind() != "source_file" {
		return ""
	}
	for _, child := range context.node.Parent().NamedChildren() {
		if child.Kind() == "part_of_directive" {
			return ""
		}
	}
	signature := context.node.ChildByFieldName("signature")
	if signature == nil {
		return ""
	}
	parameters := signature.ChildByFieldName("parameters")
	if parameters == nil || len(parameters.NamedChildren()) != 0 {
		return ""
	}
	result := signature.ChildByFieldName("return_type")
	if result != nil && result.Text() != "void" {
		return ""
	}
	return "process"
}

func dartNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, _, fields := emptyNavigationSourceFacts()
	for _, node := range root.NamedChildren() {
		if node.Kind() == "import_or_export" {
			dartNavigationImports(imports, node)
		}
	}
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, "", fields
}

func dartNavigationImports(imports map[string]navigationImport, node *syntaxNode) {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "library_import" {
			continue
		}
		for _, specification := range child.NamedChildren() {
			if specification.Kind() == "import_specification" {
				dartNavigationImportSpecification(imports, specification, child.StartLine())
			}
		}
	}
}

func dartNavigationImportSpecification(imports map[string]navigationImport, specification *syntaxNode, line int) {
	uri := dartNavigationURI(specification.ChildByFieldName("uri"))
	if uri == "" {
		return
	}
	alias := "*"
	if prefixed := specification.ChildByFieldName("alias"); prefixed != nil {
		alias = prefixed.Text()
	}
	addScopedNavigationImport(imports, alias, uri, "*", line)
}

func dartNavigationURI(node *syntaxNode) string {
	if node == nil {
		return ""
	}
	if node.Kind() != "uri" {
		for _, child := range node.NamedChildren() {
			if child.Kind() == "configuration_uri" {
				return "" // The selected Dart platform is unknown.
			}
		}
		for _, child := range node.NamedChildren() {
			if child.Kind() == "uri" {
				node = child
				break
			}
		}
	}
	if node.Kind() != "uri" {
		return ""
	}
	return dartLiteralURI(node.Text())
}

func dartLiteralURI(text string) string {
	if len(text) < 2 || (text[0] != '\'' && text[0] != '"') || text[0] != text[len(text)-1] || strings.ContainsAny(text, "$\\") {
		return ""
	}
	return text[1 : len(text)-1]
}
