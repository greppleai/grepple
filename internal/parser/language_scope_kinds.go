package parser

// codeScopeKinds is an adapter-owned grammar vocabulary, not output derived from
// arbitrary syntax node names. Keep every entry paired with a fixture in
// scope_annotations_test.go; unsupported nodes intentionally produce no label.
var codeScopeKinds = buildCodeScopeKinds()

func buildCodeScopeKinds() map[string]map[string]string {
	kinds := make(map[string]map[string]string, len(languageAdapters))
	add := func(lang, label string, nodes ...string) {
		if kinds[lang] == nil {
			kinds[lang] = make(map[string]string)
		}
		for _, node := range nodes {
			kinds[lang][node] = label
		}
	}
	// Each language opts in to control-flow nodes from its own grammar. An
	// unrecognized variant is omitted rather than labeled from its spelling.
	for _, lang := range []string{"go", "java", "dart", "swift", "javascript", "typescript", "tsx", "python", "csharp", "c", "cpp", "php", "shell"} {
		add(lang, "if", "if_statement")
		add(lang, "while", "while_statement")
		add(lang, "for", "for_statement")
		add(lang, "switch", "switch_statement")
	}
	for _, lang := range []string{"go", "java", "dart", "swift", "javascript", "typescript", "tsx", "python", "csharp", "c", "cpp", "php"} {
		add(lang, "try", "try_statement")
		add(lang, "catch", "catch_clause")
	}
	for _, lang := range []string{"javascript", "typescript", "tsx", "c", "cpp"} {
		add(lang, "for", "for_in_statement")
		add(lang, "case", "case_statement")
	}
	for _, lang := range []string{"javascript", "typescript", "tsx", "php"} {
		add(lang, "for", "for_of_statement", "foreach_statement")
		add(lang, "case", "switch_case")
	}
	add("go", "case", "expression_case", "type_case", "default_case")
	add("go", "for", "range_clause")
	add("go", "func", "function_declaration", "func_literal")
	add("go", "method", "method_declaration")
	add("go", "struct", "struct_type")
	add("go", "interface", "interface_type")

	for _, lang := range []string{"java", "csharp", "dart", "php"} {
		add(lang, "class", "class_declaration")
		add(lang, "interface", "interface_declaration")
		add(lang, "enum", "enum_declaration")
		add(lang, "method", "method_declaration", "constructor_declaration")
	}
	add("java", "class", "record_declaration")
	add("csharp", "struct", "struct_declaration", "record_declaration")
	add("csharp", "func", "local_function_statement", "lambda_expression")
	add("dart", "func", "function_declaration", "function_expression")
	add("dart", "method", "getter_declaration", "setter_declaration")
	add("dart", "class", "mixin_declaration", "extension_declaration", "extension_type_declaration")
	add("php", "func", "function_definition", "anonymous_function_creation_expression", "arrow_function")
	add("php", "class", "trait_declaration")

	for _, lang := range []string{"javascript", "typescript", "tsx"} {
		add(lang, "func", "function_declaration", "generator_function_declaration", "arrow_function", "function")
		add(lang, "method", "method_definition")
		add(lang, "class", "class_declaration", "class")
	}
	for _, lang := range []string{"typescript", "tsx"} {
		add(lang, "interface", "interface_declaration")
		add(lang, "enum", "enum_declaration")
	}
	add("kotlin", "func", "function_declaration", "anonymous_function", "lambda_literal")
	add("kotlin", "class", "class_declaration", "object_declaration")
	add("kotlin", "if", "if_expression")
	add("kotlin", "while", "while_statement")
	add("kotlin", "for", "for_statement")
	add("swift", "func", "function_declaration", "protocol_function_declaration")
	add("swift", "method", "init_declaration")
	add("swift", "class", "class_declaration")
	add("swift", "interface", "protocol_declaration")
	add("python", "func", "function_definition", "lambda")
	add("python", "class", "class_definition")
	add("python", "for", "for_in_statement")
	add("c", "func", "function_definition", "lambda_expression")
	add("cpp", "func", "function_definition", "lambda_expression")
	for _, lang := range []string{"c", "cpp"} {
		add(lang, "struct", "struct_specifier", "union_specifier")
		add(lang, "enum", "enum_specifier")
	}
	add("cpp", "class", "class_specifier")
	add("rust", "func", "function_item", "closure_expression")
	add("rust", "struct", "struct_item")
	add("rust", "interface", "trait_item")
	add("rust", "enum", "enum_item")
	add("rust", "if", "if_expression", "if_let_expression")
	add("rust", "while", "while_expression", "while_let_expression")
	add("rust", "for", "for_expression")
	add("rust", "case", "match_arm")
	add("shell", "func", "function_definition")
	add("shell", "case", "case_statement")
	add("hcl", "block", "block")
	add("css", "rule", "rule_set", "media_statement", "supports_statement", "scope_statement", "keyframes_statement", "keyframe_block")
	add("css", "property", "declaration")
	add("svelte", "if", "if_statement", "else_if_block")
	add("svelte", "for", "each_statement")
	add("svelte", "snippet", "snippet_statement")
	return kinds
}

// scopeKindForNode consults the parser-owned per-adapter grammar mapping,
// including injected Svelte JS/TS/CSS trees. Only a named member declaration
// inside a type becomes a method; lambdas/closures remain functions.
func scopeKindForNode(node ViewNode) string {
	language := node.Language()
	if node.raw != nil && node.raw.tree != nil && node.raw.tree.language != "" {
		language = node.raw.tree.language
	}
	kind := codeScopeKinds[language][node.Kind()]
	if kind != "func" || language == "go" || language == "shell" || !memberFunctionDeclaration(language, node.Kind()) {
		return kind
	}
	for parent := node.Parent(); parent.Valid(); parent = parent.Parent() {
		// An impl_item is a Rust method container, but is not a class or
		// itself a navigable callable; only its function child is a method.
		if language == "rust" && parent.Kind() == "impl_item" {
			return "method"
		}
		parentKind := codeScopeKinds[language][parent.Kind()]
		if parentKind == "func" || parentKind == "method" {
			break
		}
		if parentKind == "class" || parentKind == "struct" || parentKind == "interface" {
			return "method"
		}
	}
	return kind
}

// memberFunctionDeclaration is deliberately narrower than all callable kinds:
// a class field holding an anonymous arrow/lambda is not a method.
func memberFunctionDeclaration(language, nodeKind string) bool {
	switch language {
	case "kotlin", "swift", "dart", "javascript", "typescript", "tsx":
		return nodeKind == "function_declaration" || nodeKind == "protocol_function_declaration"
	case "python", "php", "c", "cpp":
		return nodeKind == "function_definition"
	case "rust":
		return nodeKind == "function_item"
	}
	return false
}

func navigableScopeKind(language, kind string) bool {
	switch kind {
	case "func", "method", "class", "struct", "interface", "enum", "snippet":
		return true
	case "block":
		return language == "hcl"
	case "rule":
		return language == "css"
	}
	return false
}
