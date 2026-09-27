package parser

import "strings"

func phpNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules:                 rules,
		callTypes:             newStringSet("function_call_expression", "member_call_expression", "nullsafe_member_call_expression", "scoped_call_expression", "object_creation_expression"),
		parameterTypes:        newStringSet("simple_parameter", "variadic_parameter", "property_promotion_parameter"),
		fieldDeclarationTypes: newStringSet("property_declaration"), fieldContainerTypes: rules.classDeclarationTypes,
		fieldNames: phpNavigationFieldNames, sourceFacts: phpNavigationSourceFacts, exports: phpNavigationExports,
		terminalName: phpNavigationTerminal, callDisplay: phpNavigationCallDisplay,
		selfBindingFromContainer: true, selfBindingName: "$this",
		callableReturnBinding: func(node *syntaxNode, _ string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, "", imports, "return_type")
		},
		visibility: func(node *syntaxNode, _ string, _ string) NavigationVisibility {
			if node.Kind() == "function_definition" {
				return NavigationVisibilityPublic
			}
			for _, child := range node.NamedChildren() {
				if child.Kind() == "visibility_modifier" {
					switch child.Text() {
					case "public":
						return NavigationVisibilityPublic
					case "private":
						return NavigationVisibilityNonPublic
					case "protected":
						return NavigationVisibilityNonPublic
					}
				}
			}
			return NavigationVisibilityPublic
		},
		typeReferenceFacts: true, fieldFacts: true, memberAccessFacts: true,
	}
}

func phpNavigationFieldNames(node, _ *syntaxNode, _ string) []navigationFieldName {
	var result []navigationFieldName
	for _, child := range node.NamedChildren() {
		if child.Kind() == "property_element" {
			if name := child.ChildByFieldName("name"); name != nil {
				result = append(result, navigationFieldName{name: strings.TrimPrefix(name.Text(), "$")})
			}
		}
	}
	return result
}

func phpNavigationTerminal(node *syntaxNode, _ string) string {
	if node.Kind() == "variable_name" || node.Kind() == "name" || node.Kind() == "qualified_name" || node.Kind() == "relative_name" {
		value := strings.TrimSpace(node.Text())
		if i := strings.LastIndex(value, `\`); i >= 0 {
			return value[i+1:]
		}
		return value
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() == "name" || child.Kind() == "qualified_name" {
			return phpNavigationTerminal(child, "")
		}
	}
	return ""
}

func phpNavigationCallDisplay(call, target *syntaxNode, _ string) string {
	if call.Kind() == "object_creation_expression" {
		for _, child := range call.NamedChildren() {
			if child.Kind() == "name" || child.Kind() == "qualified_name" {
				return child.Text()
			}
		}
	}
	if call.Kind() == "member_call_expression" || call.Kind() == "nullsafe_member_call_expression" || call.Kind() == "scoped_call_expression" {
		owner := call.ChildByFieldName("object")
		if owner == nil {
			owner = call.ChildByFieldName("scope")
		}
		if owner != nil {
			return owner.Text() + "." + target.Text()
		}
	}
	return target.Text()
}

func phpNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	namespaces := 0
	for _, child := range root.NamedChildren() {
		if child.Kind() == "namespace_definition" {
			namespaces++
			if name := child.ChildByFieldName("name"); name != nil {
				packageName = strings.ReplaceAll(name.Text(), `\`, ".")
			}
		}
	}
	if namespaces > 1 {
		packageName = ""
	}
	// Imports in a multi-namespace file cannot be safely made file-wide.
	if namespaces <= 1 {
		for _, child := range root.NamedChildren() {
			if child.Kind() == "namespace_use_declaration" {
				phpNavigationImport(imports, child)
			}
			if child.Kind() == "namespace_definition" {
				if body := child.ChildByFieldName("body"); body != nil {
					for _, item := range body.NamedChildren() {
						if item.Kind() == "namespace_use_declaration" {
							phpNavigationImport(imports, item)
						}
					}
				}
			}
		}
	}
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, packageName, fields
}
