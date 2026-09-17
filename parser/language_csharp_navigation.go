package parser

import "strings"

func cSharpNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("invocation_expression", "object_creation_expression"), parameterTypes: newStringSet("parameter"), sourceFacts: cSharpNavigationSourceFacts, exports: cSharpNavigationExports, entrypoint: cSharpNavigationEntrypoint, typeReferenceFacts: true, memberAccessFacts: true, visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return cSharpNavigationVisibility(navigationDeclarationHeader(node, content))
	}}
}

func cSharpNavigationEntrypoint(context navigationEntrypointContext) string {
	node := context.node
	name := node.ChildByFieldName("name")
	returnType := node.ChildByFieldName("returns")
	header := navigationDeclarationHeader(node, context.content)
	if node.Kind() != "method_declaration" || name == nil || name.Text() != "Main" || context.container == "" || returnType == nil || !navigationHeaderHasWord(header, "static") || cSharpNavigationGenericMethod(node) || !cSharpNavigationMainReturnType(returnType.Text()) || !cSharpNavigationMainParameters(node) {
		return ""
	}
	return "process"
}

func cSharpNavigationGenericMethod(node *syntaxNode) bool {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "type_parameter_list" {
			return true
		}
	}
	return false
}

func cSharpNavigationMainReturnType(value string) bool {
	switch compactNavigationQualifiedName(value) {
	case "void", "int", "System.Threading.Tasks.Task", "System.Threading.Tasks.Task<int>":
		return true
	default:
		return false
	}
}

func cSharpNavigationMainParameters(node *syntaxNode) bool {
	parameters := node.ChildByFieldName("parameters")
	if parameters == nil {
		return false
	}
	children := parameters.NamedChildren()
	if len(children) == 0 {
		return true
	}
	if len(children) != 1 || children[0].Kind() != "parameter" || navigationHeaderHasWord(children[0].Text(), "ref") || navigationHeaderHasWord(children[0].Text(), "out") || navigationHeaderHasWord(children[0].Text(), "in") || navigationHeaderHasWord(children[0].Text(), "params") {
		return false
	}
	typeNode := children[0].ChildByFieldName("type")
	if typeNode == nil {
		return false
	}
	typeName := compactNavigationQualifiedName(typeNode.Text())
	return typeName == "string[]" || typeName == "System.String[]"
}

func cSharpNavigationSourceFacts(root *syntaxNode, _ string, _ *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	packageName, scope, unambiguous := cSharpNavigationScope(root)
	addCSharpNavigationUsings(imports, root)
	if unambiguous && scope != nil && scope.ID() != root.ID() {
		addCSharpNavigationUsings(imports, scope)
	}
	return imports, packageName, fields
}

func cSharpNavigationScope(root *syntaxNode) (string, *syntaxNode, bool) {
	var namespace *syntaxNode
	count := 0
	for _, node := range root.NamedChildren() {
		if node.Kind() != "namespace_declaration" && node.Kind() != "file_scoped_namespace_declaration" {
			continue
		}
		namespace = node
		count++
	}
	if count == 0 {
		return "", root, true
	}
	if count != 1 || namespace == nil {
		return "", root, false
	}
	name := navigationFieldText(namespace, "name", "")
	if namespace.Kind() == "file_scoped_namespace_declaration" {
		return compactNavigationQualifiedName(name), root, true
	}
	body := namespace.ChildByFieldName("body")
	if body == nil {
		return "", root, false
	}
	return compactNavigationQualifiedName(name), body, true
}

func addCSharpNavigationUsings(imports map[string]navigationImport, scope *syntaxNode) {
	for _, node := range scope.NamedChildren() {
		if node.Kind() == "using_directive" {
			addCSharpNavigationUsing(imports, node)
		}
	}
}

func addCSharpNavigationUsing(imports map[string]navigationImport, node *syntaxNode) {
	value := strings.TrimSpace(strings.TrimSuffix(node.Text(), ";"))
	value, _ = trimNavigationDirectiveKeyword(value, "global")
	value, _ = trimNavigationDirectiveKeyword(value, "using")
	var static bool
	value, static = trimNavigationDirectiveKeyword(value, "static")
	alias := ""
	if before, after, ok := strings.Cut(value, "="); ok {
		alias, value = strings.TrimSpace(before), strings.TrimSpace(after)
	}
	target, ok := cSharpNavigationImportTarget(value)
	if !ok {
		return
	}
	if alias != "" {
		// C# aliases may name either a namespace or a type. Keep the fact, but
		// do not promote the alias to a call binding without repository evidence.
		addUnboundNavigationImport(imports, alias, target, navigationTerminal(target), node.StartLine())
		return
	}
	imported := "*"
	if static {
		imported = ""
	}
	addScopedNavigationImport(imports, "*", target, imported, node.StartLine())
}

func trimNavigationDirectiveKeyword(value, keyword string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == keyword {
		return "", true
	}
	if !strings.HasPrefix(value, keyword) || len(value) == len(keyword) || !strings.ContainsRune(" \t\r\n", rune(value[len(keyword)])) {
		return value, false
	}
	return strings.TrimSpace(value[len(keyword):]), true
}

func cSharpNavigationImportTarget(value string) (string, bool) {
	target := compactNavigationQualifiedName(strings.TrimPrefix(strings.TrimSpace(value), "global::"))
	if target == "" || strings.Contains(target, "::") || strings.ContainsAny(target, "<>[](),?*") {
		return "", false
	}
	return target, true
}

func cSharpNavigationExports(root *syntaxNode, content, language, path string) []NavigationExport {
	packageName, scope, unambiguous := cSharpNavigationScope(root)
	if !unambiguous || scope == nil {
		return nil
	}
	exports := []NavigationExport{}
	for _, node := range scope.NamedChildren() {
		if !cSharpTopLevelExportKind(node.Kind()) || cSharpNavigationVisibility(navigationDeclarationHeader(node, content)) != NavigationVisibilityPublic {
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

func cSharpTopLevelExportKind(kind string) bool {
	switch kind {
	case "class_declaration", "interface_declaration", "struct_declaration", "record_declaration", "enum_declaration", "delegate_declaration":
		return true
	}
	return false
}
