package parser

import "strings"

func phpNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules:                 rules,
		callTypes:             newStringSet("function_call_expression", "member_call_expression", "nullsafe_member_call_expression", "scoped_call_expression", "object_creation_expression"),
		parameterTypes:        newStringSet("simple_parameter", "variadic_parameter", "property_promotion_parameter"),
		fieldDeclarationTypes: newStringSet("property_declaration"), fieldContainerTypes: rules.classDeclarationTypes,
		fieldNames:      phpFieldNames,
		selfBindingName: "$this", selfBindingFromContainer: true,
		terminalName:       phpTerminalName,
		callDisplay:        phpCallDisplay,
		sourceFacts:        phpSourceFacts,
		exports:            phpExports,
		typeReferenceFacts: true, fieldFacts: true, memberAccessFacts: true,
		visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
			header := navigationDeclarationHeader(node, content)
			if content == "" {
				header = node.Text()
			}
			if navigationHeaderHasWord(header, "private") || navigationHeaderHasWord(header, "protected") {
				return NavigationVisibilityNonPublic
			}
			return NavigationVisibilityPublic
		},
	}
}

func phpFieldNames(node, typeNode *syntaxNode, _ string) []navigationFieldName {
	var names []navigationFieldName
	for _, child := range node.NamedChildren() {
		if child.ID() == typeNode.ID() {
			continue
		}
		if child.Kind() == "property_element" {
			for _, item := range child.NamedChildren() {
				if item.Kind() == "variable_name" {
					names = append(names, navigationFieldName{name: strings.TrimPrefix(item.Text(), "$")})
					break
				}
			}
		}
	}
	return names
}

func phpTerminalName(node *syntaxNode, _ string) string {
	if name := navigationFirstField(node, "name", "function"); name != nil {
		return strings.TrimPrefix(phpQualifiedTerminal(name.Text()), "$")
	}
	return strings.TrimPrefix(phpQualifiedTerminal(node.Text()), "$")
}

func phpQualifiedTerminal(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndex(name, "\\"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func phpCallDisplay(call, target *syntaxNode, _ string) string {
	if object := navigationFirstField(call, "object", "scope"); object != nil {
		return object.Text() + "::" + target.Text()
	}
	return target.Text()
}

func phpSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	var namespace *syntaxNode
	for _, child := range root.NamedChildren() {
		if child.Kind() != "namespace_definition" {
			continue
		}
		if namespace != nil {
			// PHP permits several independent namespaces in one file. The shared
			// graph has one file-wide package/import context, so do not merge
			// aliases from unrelated namespaces into a false binding.
			collectNavigationSourceFields(root, content, imports, fields, adapter)
			return imports, "", fields
		}
		namespace = child
	}
	if namespace != nil {
		if name := namespace.ChildByFieldName("name"); name != nil {
			packageName = strings.TrimPrefix(name.Text(), "\\")
		}
		if body := namespace.ChildByFieldName("body"); body != nil {
			phpCollectUses(imports, body)
		}
	}
	phpCollectUses(imports, root)
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	return imports, packageName, fields
}

func phpCollectUses(imports map[string]navigationImport, scope *syntaxNode) {
	for _, node := range scope.NamedChildren() {
		if node.Kind() != "namespace_use_declaration" {
			continue
		}
		for _, clause := range node.NamedChildren() {
			if clause.Kind() != "namespace_use_clause" {
				continue
			}
			var target string
			for _, name := range clause.NamedChildren() {
				if name.Kind() == "qualified_name" || name.Kind() == "name" {
					target = strings.TrimPrefix(name.Text(), "\\")
					break
				}
			}
			if target == "" {
				continue
			}
			alias := phpQualifiedTerminal(target)
			if named := clause.ChildByFieldName("alias"); named != nil {
				alias = named.Text()
			}
			addScopedNavigationImport(imports, alias, target, phpQualifiedTerminal(target), node.StartLine())
		}
	}
}
