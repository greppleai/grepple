package parser

import "strings"

func javaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules:       rules,
		callTypes:   newStringSet("method_invocation", "object_creation_expression", "explicit_constructor_invocation"),
		sourceFacts: javaNavigationSourceFacts,
		callDisplay: javaNavigationCallDisplay,
		exports:     jvmNavigationExports,
		visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
			return javaNavigationVisibility(node, navigationDeclarationHeader(node, content))
		},
	}
}

func javaNavigationCallDisplay(call, target *syntaxNode, _ string) string {
	object := call.ChildByFieldName("object")
	if object == nil {
		return target.Text()
	}
	return object.Text() + "." + target.Text()
}

func kotlinNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), sourceFacts: kotlinNavigationSourceFacts, exports: jvmNavigationExports, visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return visibilityFromModifiers(navigationDeclarationHeader(node, content), true)
	}}
}

func javaNavigationSourceFacts(root *syntaxNode, _ string, _ *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	for _, node := range root.NamedChildren() {
		switch node.Kind() {
		case "package_declaration":
			packageName = compactNavigationQualifiedName(navigationDirectiveValue(node.Text(), "package"))
		case "import_declaration":
			parts := strings.Fields(navigationDirectiveValue(node.Text(), "import"))
			static := len(parts) > 1 && parts[0] == "static"
			if static {
				parts = parts[1:]
			}
			addJVMNavigationImport(imports, compactNavigationQualifiedName(strings.Join(parts, "")), "", static, node.StartLine())
		}
	}
	return imports, packageName, fields
}

func kotlinNavigationSourceFacts(root *syntaxNode, _ string, _ *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	for _, node := range root.NamedChildren() {
		switch node.Kind() {
		case "package_header":
			packageName = compactNavigationQualifiedName(navigationDirectiveValue(node.Text(), "package"))
		case "import":
			value := navigationDirectiveValue(node.Text(), "import")
			parts := strings.Fields(value)
			alias := ""
			if len(parts) >= 3 && parts[len(parts)-2] == "as" {
				alias = parts[len(parts)-1]
				parts = parts[:len(parts)-2]
			}
			addJVMNavigationImport(imports, compactNavigationQualifiedName(strings.Join(parts, "")), alias, false, node.StartLine())
		}
	}
	return imports, packageName, fields
}

func addJVMNavigationImport(imports map[string]navigationImport, value, alias string, static bool, line int) {
	if value == "" {
		return
	}
	if strings.HasSuffix(value, ".*") {
		addScopedNavigationImport(imports, "*", strings.TrimSuffix(value, ".*"), "*", line)
		return
	}
	imported := navigationTerminal(value)
	importPath := value
	if static {
		importPath = strings.TrimSuffix(value, "."+imported)
	}
	if alias == "" {
		alias = imported
	}
	addScopedNavigationImport(imports, alias, importPath, imported, line)
}

func jvmNavigationExports(root *syntaxNode, content, language, path string) []NavigationExport {
	packageName := ""
	packageKind := "package_declaration"
	if language == "kotlin" {
		packageKind = "package_header"
	}
	for _, node := range root.NamedChildren() {
		if node.Kind() == packageKind {
			packageName = compactNavigationQualifiedName(navigationDirectiveValue(node.Text(), "package"))
			break
		}
	}
	exports := []NavigationExport{}
	for _, node := range root.NamedChildren() {
		if !jvmTopLevelExportKind(node.Kind(), language) || jvmNavigationExportVisibility(node, content, language) != NavigationVisibilityPublic {
			continue
		}
		name := node.ChildByFieldName("name")
		if name == nil || strings.TrimSpace(name.Text()) == "" {
			continue
		}
		exports = append(exports, NavigationExport{Name: strings.TrimSpace(name.Text()), LocalName: strings.TrimSpace(name.Text()), ImportPath: packageName, Language: language, Path: path, Line: node.StartLine()})
	}
	return exports
}

func jvmNavigationExportVisibility(node *syntaxNode, content, language string) NavigationVisibility {
	header := navigationDeclarationHeader(node, content)
	if language == "java" {
		return javaNavigationVisibility(node, header)
	}
	return visibilityFromModifiers(header, true)
}

func jvmTopLevelExportKind(kind, language string) bool {
	if language == "java" {
		switch kind {
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration":
			return true
		}
		return false
	}
	switch kind {
	case "class_declaration", "object_declaration", "function_declaration", "type_alias":
		return true
	}
	return false
}
