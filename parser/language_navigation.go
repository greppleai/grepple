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

func goNavigationEntrypoint(node *syntaxNode, name, container, packageName, _ string) string {
	if node.Kind() == "function_declaration" && name == "main" && container == "" && packageName == "main" {
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

func ecmaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("call_expression", "new_expression"), extraContainerTypes: newStringSet("interface_declaration"),
		fieldContainerTypes: newStringSet("class_declaration", "interface_declaration"), selfBindingName: "this", selfBindingFromContainer: true,
		visibility: typeScriptAdapterVisibility, sourceFacts: typeScriptNavigationSourceFacts, exports: typeScriptNavigationExports,
		callableReturnBinding: func(node *syntaxNode, content string, imports map[string]navigationImport) navigationBinding {
			return navigationReturnBindingFromFields(node, content, imports, "return_type", "type")
		},
	}
}

func pythonNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call"), sourceFacts: pythonNavigationSourceFacts, visibility: func(_ *syntaxNode, name, _ string) NavigationVisibility {
		return pythonNavigationVisibility(navigationTerminal(name))
	}}
}

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

func cSharpNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("invocation_expression", "object_creation_expression"), sourceFacts: cSharpNavigationSourceFacts, exports: cSharpNavigationExports, visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return cSharpNavigationVisibility(navigationDeclarationHeader(node, content))
	}}
}

func cFamilyNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), declarationName: func(node *syntaxNode, content string, _ *navigationEnvelope) string {
		return cFamilyName(node, content, rules)
	}}
}

func rustNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), sourceFacts: rustNavigationSourceFacts, exports: rustNavigationExports, nestedModulePath: func(node *syntaxNode, current string) string {
		if node.Kind() != "mod_item" || node.ChildByFieldName("body") == nil {
			return ""
		}
		return rustNavigationJoinPath(current, navigationFieldText(node, "name", ""))
	}, containerName: func(node *syntaxNode, content string, envelope *navigationEnvelope) string {
		if node.Kind() == "impl_item" {
			if target := node.ChildByFieldName("type"); target != nil {
				return target.Text()
			}
		}
		return defaultNavigationDeclarationName(node, content, rules, envelope)
	}, visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return visibilityFromRequiredModifier(navigationDeclarationHeader(node, content), "pub")
	}, visibilityDetail: rustNavigationVisibilityDetail}
}

func shellNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("command")}
}

func defaultNavigationDeclarationName(node *syntaxNode, content string, rules *structureRules, envelope *navigationEnvelope) string {
	if envelope != nil && envelope.assignedName != "" {
		return envelope.assignedName
	}
	return extractNodeName(node, content, rules)
}

func defaultNavigationDeclarationKind(node *syntaxNode, container string) string {
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

func defaultNavigationTerminalName(node *syntaxNode) string {
	candidate := ""
	node.WalkNamed(func(current *syntaxNode) {
		switch current.Kind() {
		case "identifier", "field_identifier", "property_identifier", "type_identifier", "command_name", "word":
			candidate = current.Text()
		}
	})
	return candidate
}

var defaultNavigationWrapperTypes = newStringSet("export_statement", "decorated_definition", "lexical_declaration", "variable_declaration", "variable_declarator", "template_declaration")

var defaultNavigationNestedBindingScopes = newStringSet(
	"block", "statement_block", "if_statement", "for_statement", "for_in_statement", "while_statement", "do_statement",
	"switch_statement", "expression_switch_statement", "type_switch_statement", "select_statement", "try_statement", "catch_clause",
	"finally_clause", "expression_case", "type_case", "communication_case", "switch_case", "switch_default", "case_clause", "default_clause",
)

var defaultNavigationMemberTypes = newStringSet(
	"selector_expression", "member_expression", "attribute", "field_access", "member_access_expression", "field_expression", "navigation_expression",
)

var defaultNavigationFieldDeclarationTypes = newStringSet(
	"field_declaration", "public_field_definition", "field_definition", "property_signature", "abstract_property_signature",
)

var defaultNavigationParameterTypes = newStringSet(
	"parameter_declaration", "variadic_parameter_declaration", "required_parameter", "optional_parameter",
)

var defaultNavigationVariableBindingKinds = map[string]string{
	"variable_declarator":   "typed",
	"var_spec":              "typed",
	"short_var_declaration": "short",
}

func defaultNavigationParameterNames(node, typeNode *syntaxNode, _ string) []string {
	if name := node.ChildByFieldName("name"); name != nil {
		return []string{name.Text()}
	}
	if pattern := node.ChildByFieldName("pattern"); pattern != nil {
		return []string{pattern.Text()}
	}
	var names []string
	for _, child := range node.NamedChildren() {
		if child.ID() != typeNode.ID() && (child.Kind() == "identifier" || child.Kind() == "field_identifier") {
			names = append(names, child.Text())
		}
	}
	return names
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

func typeScriptNavigationSourceFacts(root *syntaxNode, content string, adapter *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	root.WalkNamed(func(node *syntaxNode) {
		if node.Kind() == "import_statement" {
			addTypeScriptNavigationImports(imports, node, content)
		}
	})
	collectNavigationSourceFields(root, content, imports, fields, adapter)
	collectTypeScriptNavigationHeritage(root, imports, fields)
	return imports, packageName, fields
}

func pythonNavigationSourceFacts(root *syntaxNode, _ string, _ *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	for _, node := range root.NamedChildren() {
		switch node.Kind() {
		case "import_statement":
			addPythonNavigationImports(imports, node)
		case "import_from_statement":
			addPythonNavigationFromImports(imports, node)
		}
	}
	return imports, packageName, fields
}

func addPythonNavigationImports(imports map[string]navigationImport, node *syntaxNode) {
	for index, child := range node.NamedChildren() {
		if node.FieldNameForNamedChild(uint32(index)) != "name" {
			continue
		}
		importPath, alias := pythonNavigationImportName(child)
		if importPath == "" {
			continue
		}
		if alias == "" {
			alias = strings.Split(importPath, ".")[0]
		}
		addScopedNavigationImport(imports, alias, importPath, "*", node.StartLine())
	}
}

func addPythonNavigationFromImports(imports map[string]navigationImport, node *syntaxNode) {
	module := node.ChildByFieldName("module_name")
	if module == nil || strings.TrimSpace(module.Text()) == "" {
		return
	}
	importPath := strings.TrimSpace(module.Text())
	for index, child := range node.NamedChildren() {
		if child.ID() == module.ID() {
			continue
		}
		field := node.FieldNameForNamedChild(uint32(index))
		if field != "name" && child.Kind() != "wildcard_import" {
			continue
		}
		imported, alias := pythonNavigationImportName(child)
		if child.Kind() == "wildcard_import" {
			imported, alias = "*", "*"
		} else if alias == "" {
			alias = navigationTerminal(imported)
		}
		addScopedNavigationImport(imports, alias, importPath, imported, node.StartLine())
	}
}

func pythonNavigationImportName(node *syntaxNode) (string, string) {
	if node == nil {
		return "", ""
	}
	if node.Kind() != "aliased_import" {
		return strings.TrimSpace(node.Text()), ""
	}
	name := navigationFirstField(node, "name")
	alias := navigationFirstField(node, "alias")
	if name == nil || alias == nil {
		return "", ""
	}
	return strings.TrimSpace(name.Text()), strings.TrimSpace(alias.Text())
}

func addScopedNavigationImport(imports map[string]navigationImport, alias, importPath, imported string, line int) {
	if alias == "" || importPath == "" {
		return
	}
	item := navigationImport{alias: alias, path: importPath, imported: imported, line: line}
	if alias == "*" {
		imports[navigationImportUniqueKey(item)] = item
		return
	}
	if existing, ok := imports[alias]; ok {
		delete(imports, alias)
		imports[navigationImportUniqueKey(existing)] = existing
		imports[navigationImportUniqueKey(item)] = item
		return
	}
	for key := range imports {
		if strings.HasPrefix(key, alias+"\x00") {
			imports[navigationImportUniqueKey(item)] = item
			return
		}
	}
	imports[alias] = item
}

func addUnboundNavigationImport(imports map[string]navigationImport, alias, importPath, imported string, line int) {
	if alias == "" || importPath == "" {
		return
	}
	item := navigationImport{alias: alias, path: importPath, imported: imported, line: line}
	imports[navigationImportUniqueKey(item)] = item
}

func navigationImportUniqueKey(item navigationImport) string {
	return item.scope + "\x00" + item.alias + "\x00" + strconv.Itoa(item.line) + "\x00" + item.path + "\x00" + item.imported + "\x00" + item.kind
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

func navigationDirectiveValue(text, keyword string) string {
	value := strings.TrimSpace(text)
	if strings.HasPrefix(value, keyword) {
		value = strings.TrimSpace(strings.TrimPrefix(value, keyword))
	}
	return strings.TrimSpace(strings.TrimSuffix(value, ";"))
}

func compactNavigationQualifiedName(value string) string {
	return strings.Join(strings.Fields(value), "")
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

func collectTypeScriptNavigationHeritage(root *syntaxNode, imports map[string]navigationImport, fields map[string]map[string]navigationBinding) {
	root.WalkNamed(func(node *syntaxNode) {
		addTypeScriptNavigationHeritage(node, imports, fields)
	})
}

func addTypeScriptNavigationHeritage(node *syntaxNode, imports map[string]navigationImport, fields map[string]map[string]navigationBinding) {
	if node.Kind() != "class_declaration" && node.Kind() != "interface_declaration" {
		return
	}
	name := navigationFieldText(node, "name", "")
	if name == "" {
		return
	}
	for _, child := range node.NamedChildren() {
		if child.Kind() != "class_heritage" {
			continue
		}
		for _, clause := range child.NamedChildren() {
			addTypeScriptNavigationHeritageClause(name, clause, imports, fields)
		}
	}
}

func addTypeScriptNavigationHeritageClause(name string, clause *syntaxNode, imports map[string]navigationImport, fields map[string]map[string]navigationBinding) {
	if clause.Kind() != "extends_clause" && clause.Kind() != "implements_clause" {
		return
	}
	for _, typeNode := range clause.NamedChildren() {
		binding := navigationBindingForType(typeNode.Text(), imports)
		if binding.typeName == "" {
			continue
		}
		binding.embedded = true
		binding.line = clause.StartLine()
		if fields[name] == nil {
			fields[name] = make(map[string]navigationBinding)
		}
		fields[name][binding.typeName] = binding
	}
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

func collectNavigationSourceFields(root *syntaxNode, content string, imports map[string]navigationImport, fields map[string]map[string]navigationBinding, adapter navigationAdapter) {
	root.WalkNamed(func(node *syntaxNode) {
		addNavigationContainerFields(fields, node, content, imports, adapter)
	})
}

func typeScriptAdapterVisibility(node *syntaxNode, name, content string) NavigationVisibility {
	return typeScriptNavigationVisibility(node, navigationTerminal(name), navigationDeclarationHeader(node, content))
}

func navigationReturnBindingFromFields(node *syntaxNode, _ string, imports map[string]navigationImport, fields ...string) navigationBinding {
	var result *syntaxNode
	for _, field := range fields {
		result = node.ChildByFieldName(field)
		if result != nil {
			break
		}
	}
	if result == nil {
		return navigationBinding{}
	}
	if result.Kind() == "parameter_list" || result.Kind() == "formal_parameters" {
		children := result.NamedChildren()
		if len(children) != 1 {
			return navigationBinding{}
		}
		result = children[0]
		if typed := result.ChildByFieldName("type"); typed != nil {
			result = typed
		}
	}
	return navigationBindingForType(result.Text(), imports)
}

func navigationPackageName(node *syntaxNode, _ string) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "package_identifier" || child.Kind() == "identifier" {
			return child.Text()
		}
	}
	return ""
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

func addTypeScriptNavigationImports(imports map[string]navigationImport, node *syntaxNode, content string) {
	source := node.ChildByFieldName("source")
	if source == nil {
		return
	}
	importPath := unquoteNavigationPath(source.Text())
	addTypeScriptDefaultNavigationImport(imports, node, importPath)
	node.WalkNamed(func(current *syntaxNode) {
		switch current.Kind() {
		case "import_specifier":
			imported := navigationFieldText(current, "name", content)
			local := navigationFieldText(current, "alias", content)
			if local == "" {
				local = imported
			}
			if local != "" {
				imports[local] = navigationImport{alias: local, path: importPath, imported: imported, line: node.StartLine()}
			}
		case "namespace_import":
			if local := navigationFirstIdentifier(current, content); local != "" {
				imports[local] = navigationImport{alias: local, path: importPath, imported: "*", line: node.StartLine()}
			}
		}
	})
}

func typeScriptNavigationExports(root *syntaxNode, _ string, language, path string) []NavigationExport {
	exports := []NavigationExport{}
	root.WalkNamed(func(node *syntaxNode) {
		exports = append(exports, typeScriptNavigationExportsForNode(node, language, path)...)
	})
	return exports
}

func typeScriptNavigationExportsForNode(node *syntaxNode, language, path string) []NavigationExport {
	if node.Kind() != "export_statement" {
		return nil
	}
	source := node.ChildByFieldName("source")
	importPath := ""
	if source != nil {
		importPath = unquoteNavigationPath(source.Text())
	}
	exports := typeScriptNavigationExportSpecifiers(node, importPath, language, path)
	if len(exports) > 0 {
		return exports
	}
	if source != nil {
		return []NavigationExport{{Name: "*", ImportPath: importPath, ImportedName: "*", Language: language, Path: path, Line: node.StartLine()}}
	}
	if exported, ok := typeScriptDefaultNavigationExport(node, language, path); ok {
		return []NavigationExport{exported}
	}
	return nil
}

func typeScriptNavigationExportSpecifiers(node *syntaxNode, importPath, language, path string) []NavigationExport {
	exports := []NavigationExport{}
	node.WalkNamed(func(current *syntaxNode) {
		if current.Kind() != "export_specifier" {
			return
		}
		local := navigationFieldText(current, "name", "")
		exported := navigationFieldText(current, "alias", "")
		if exported == "" {
			exported = local
		}
		exports = append(exports, NavigationExport{Name: exported, LocalName: local, ImportPath: importPath, ImportedName: local, Language: language, Path: path, Line: current.StartLine()})
	})
	return exports
}

func typeScriptDefaultNavigationExport(node *syntaxNode, language, path string) (NavigationExport, bool) {
	if !strings.HasPrefix(strings.TrimSpace(node.Text()), "export default ") {
		return NavigationExport{}, false
	}
	for _, child := range node.NamedChildren() {
		name := navigationFieldText(child, "name", "")
		if name != "" {
			return NavigationExport{Name: "default", LocalName: name, ImportedName: name, Language: language, Path: path, Line: node.StartLine()}, true
		}
	}
	return NavigationExport{}, false
}

func addTypeScriptDefaultNavigationImport(imports map[string]navigationImport, node *syntaxNode, importPath string) {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "import_clause" {
			continue
		}
		for _, imported := range child.NamedChildren() {
			if imported.Kind() == "identifier" {
				local := imported.Text()
				imports[local] = navigationImport{alias: local, path: importPath, imported: "default", line: node.StartLine()}
			}
		}
		return
	}
}

func navigationFirstIdentifier(node *syntaxNode, _ string) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "identifier" {
			return child.Text()
		}
	}
	return ""
}

func unquoteNavigationPath(value string) string {
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, "'\"`")
}
