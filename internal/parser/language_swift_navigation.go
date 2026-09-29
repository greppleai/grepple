package parser

import "strings"

func swiftNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression"),
		fieldContainerTypes:   rules.classDeclarationTypes,
		fieldDeclarationTypes: newStringSet("property_declaration", "protocol_property_declaration"),
		parameterTypes:        newStringSet("parameter"),
		parameterType:         swiftParameterType, fieldType: swiftPropertyType,
		fieldNames: swiftPropertyNames, fieldFacts: true, typeReferenceFacts: true,
		selfBindingName: "self", selfBindingFromContainer: true,
		declarationName: func(node *syntaxNode, _ string, _ *navigationEnvelope) string { return swiftDeclarationName(node) },
		declarationKind: func(node *syntaxNode, container string) string {
			if node.Kind() == "init_declaration" {
				return "constructor"
			}
			return defaultNavigationDeclarationKind(node, container)
		},
		terminalName: swiftTerminalName,
		callableReturnBinding: func(node *syntaxNode, _ string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, "", imports, "return_type")
		},
		sourceFacts: swiftSourceFacts,
		visibility:  swiftVisibility,
	}
}

func swiftTerminalName(node *syntaxNode, _ string) string {
	if node.Kind() == "simple_identifier" || node.Kind() == "type_identifier" {
		return node.Text()
	}
	var name string
	node.WalkNamed(func(child *syntaxNode) {
		if child.Kind() == "simple_identifier" || child.Kind() == "type_identifier" {
			name = child.Text()
		}
	})
	return name
}

func swiftParameterType(node *syntaxNode) *syntaxNode { return node.ChildByFieldName("type") }

func swiftPropertyType(node *syntaxNode) *syntaxNode {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "type_annotation" {
			continue
		}
		if typed := navigationFirstField(child, "type", "name"); typed != nil {
			return typed
		}
		if child.NamedChildCount() == 1 {
			return child.NamedChild(0)
		}
	}
	return nil // Inferred properties cannot supply a reliable field type.
}

func swiftPropertyNames(node, _ *syntaxNode, _ string) []navigationFieldName {
	name := node.ChildByFieldName("name")
	if name == nil {
		return nil
	}
	if name.Kind() == "pattern" {
		name = name.ChildByFieldName("bound_identifier")
	}
	if name == nil || (name.Kind() != "simple_identifier" && name.Kind() != "identifier") {
		return nil
	}
	return []navigationFieldName{{name: name.Text()}}
}

func swiftVisibility(node *syntaxNode, _ string, _ string) NavigationVisibility {
	if explicit := swiftExplicitVisibility(node); explicit != NavigationVisibilityUnknown {
		return explicit
	}
	// Requirements of an explicitly public protocol are public as well.
	if body := node.Parent(); body != nil && body.Kind() == "protocol_body" {
		if protocol := body.Parent(); protocol != nil && protocol.Kind() == "protocol_declaration" {
			return swiftExplicitVisibility(protocol)
		}
	}
	return NavigationVisibilityUnknown // Swift's default internal scope needs target-module evidence.
}

func swiftExplicitVisibility(node *syntaxNode) NavigationVisibility {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "modifiers" {
			continue
		}
		for _, modifier := range child.NamedChildren() {
			if modifier.Kind() != "visibility_modifier" {
				continue
			}
			switch strings.TrimSpace(modifier.Text()) {
			case "public", "open":
				return NavigationVisibilityPublic
			case "private", "fileprivate":
				return NavigationVisibilityNonPublic
			}
		}
	}
	return NavigationVisibilityUnknown
}

func swiftSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, _, fields := emptyNavigationSourceFacts()
	for _, node := range root.NamedChildren() {
		if node.Kind() != "import_declaration" {
			continue
		}
		for _, name := range node.NamedChildren() {
			if name.Kind() == "identifier" {
				addScopedNavigationImport(imports, "*", name.Text(), "*", node.StartLine())
				break
			}
		}
	}
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, "", fields
}
