package parser

import (
	"path/filepath"
	"strings"
)

func javaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules:       rules,
		callTypes:   newStringSet("method_invocation", "object_creation_expression", "explicit_constructor_invocation"),
		sourceFacts: javaNavigationSourceFacts,
		callDisplay: javaNavigationCallDisplay,
		exports:     jvmNavigationExports,
		entrypoint:  javaNavigationEntrypoint,
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
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), sourceFacts: kotlinNavigationSourceFacts, exports: jvmNavigationExports, entrypoint: kotlinNavigationEntrypoint, visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return visibilityFromModifiers(navigationDeclarationHeader(node, content), true)
	}}
}

func javaNavigationEntrypoint(context navigationEntrypointContext) string {
	node := context.node
	name := node.ChildByFieldName("name")
	typeNode := node.ChildByFieldName("type")
	header := navigationDeclarationHeader(node, context.content)
	if node.Kind() != "method_declaration" || name == nil || name.Text() != "main" || context.container == "" || typeNode == nil || typeNode.Text() != "void" || !navigationHeaderHasWord(header, "public") || !navigationHeaderHasWord(header, "static") || !javaNavigationMainParameters(node) {
		return ""
	}
	return "process"
}

func javaNavigationMainParameters(node *syntaxNode) bool {
	parameters := node.ChildByFieldName("parameters")
	if parameters == nil {
		return false
	}
	children := parameters.NamedChildren()
	if len(children) != 1 {
		return false
	}
	parameter := children[0]
	switch parameter.Kind() {
	case "formal_parameter":
		typeNode := parameter.ChildByFieldName("type")
		if typeNode == nil {
			return false
		}
		typeName := compactNavigationQualifiedName(typeNode.Text())
		return typeName == "String[]" || typeName == "java.lang.String[]"
	case "spread_parameter":
		for _, child := range parameter.NamedChildren() {
			if child.Kind() != "type_identifier" && child.Kind() != "scoped_type_identifier" {
				continue
			}
			typeName := compactNavigationQualifiedName(child.Text())
			return typeName == "String" || typeName == "java.lang.String"
		}
	}
	return false
}

func kotlinNavigationEntrypoint(context navigationEntrypointContext) string {
	node := context.node
	name := node.ChildByFieldName("name")
	parent := node.Parent()
	header := navigationDeclarationHeader(node, context.content)
	if node.Kind() != "function_declaration" || name == nil || name.Text() != "main" || parent == nil || parent.Kind() != "source_file" || strings.EqualFold(filepath.Ext(context.path), ".kts") || visibilityFromModifiers(header, true) != NavigationVisibilityPublic || navigationHeaderHasWord(header, "JvmName") {
		return ""
	}
	parameters, returnType, blockBody := kotlinNavigationFunctionSignature(node)
	if parameters == nil || !kotlinNavigationMainParameters(parameters) {
		return ""
	}
	if returnType != "" && returnType != "Unit" && returnType != "kotlin.Unit" {
		return ""
	}
	if returnType == "" && !blockBody {
		return ""
	}
	return "process"
}

func kotlinNavigationFunctionSignature(node *syntaxNode) (*syntaxNode, string, bool) {
	var parameters *syntaxNode
	returnType := ""
	blockBody := false
	for _, child := range node.NamedChildren() {
		switch child.Kind() {
		case "function_value_parameters":
			parameters = child
		case "user_type", "nullable_type", "function_type", "parenthesized_type", "type_identifier", "dynamic_type":
			if parameters != nil && returnType == "" {
				returnType = compactNavigationQualifiedName(child.Text())
			}
		case "function_body":
			for _, bodyChild := range child.NamedChildren() {
				if bodyChild.Kind() == "block" {
					blockBody = true
					break
				}
			}
		}
	}
	return parameters, returnType, blockBody
}

func kotlinNavigationMainParameters(parameters *syntaxNode) bool {
	children := parameters.NamedChildren()
	if len(children) == 0 {
		return true
	}
	if len(children) != 1 || children[0].Kind() != "parameter" {
		return false
	}
	for _, child := range children[0].NamedChildren() {
		if child.Kind() == "user_type" {
			typeName := compactNavigationQualifiedName(child.Text())
			return typeName == "Array<String>" || typeName == "kotlin.Array<kotlin.String>"
		}
	}
	return false
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
