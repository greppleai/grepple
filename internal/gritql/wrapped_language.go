package gritql

import (
	"fmt"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
)

const (
	// CGrammar identifies the C syntax contract.
	CGrammar = "c"
	// TreeSitterCGrammar identifies the pinned C grammar implementation.
	TreeSitterCGrammar = "tree-sitter-c@0.24.2"
	// CPPGrammar identifies the C++ syntax contract.
	CPPGrammar = "cpp"
	// TreeSitterCPPGrammar identifies the pinned C++ grammar implementation.
	TreeSitterCPPGrammar = "tree-sitter-cpp@0.23.4"
	// CSharpGrammar identifies the C# syntax contract.
	CSharpGrammar = "csharp"
	// TreeSitterCSharpGrammar identifies the pinned C# grammar implementation.
	TreeSitterCSharpGrammar = "tree-sitter-c-sharp@0.23.4"
	// JavaGrammar identifies the Java syntax contract.
	JavaGrammar = "java"
	// TreeSitterJavaGrammar identifies the pinned Java grammar implementation.
	TreeSitterJavaGrammar = "tree-sitter-java@0.23.5"
	// DartGrammar identifies the Dart syntax contract.
	DartGrammar = "dart"
	// TreeSitterDartGrammar identifies the pinned Dart grammar implementation.
	TreeSitterDartGrammar = "tree-sitter-dart@0.2.0"
	// SwiftGrammar identifies the Swift syntax contract.
	SwiftGrammar = "swift"
	// TreeSitterSwiftGrammar identifies the pinned Swift grammar implementation.
	TreeSitterSwiftGrammar = "tree-sitter-swift@88bfd19a89be"
	// KotlinGrammar identifies the Kotlin syntax contract.
	KotlinGrammar = "kotlin"
	// TreeSitterKotlinGrammar identifies the pinned Kotlin grammar implementation.
	TreeSitterKotlinGrammar = "tree-sitter-kotlin@1.1.0"
	// RustGrammar identifies the Rust syntax contract.
	RustGrammar = "rust"
	// TreeSitterRustGrammar identifies the pinned Rust grammar implementation.
	TreeSitterRustGrammar = "tree-sitter-rust@0.24.2"
	// PHPGrammar identifies the PHP syntax contract.
	PHPGrammar = "php"
	// TreeSitterPHPGrammar identifies the pinned PHP grammar implementation.
	TreeSitterPHPGrammar = "tree-sitter-php@0.25.0"
	// ShellGrammar identifies the Shell syntax contract.
	ShellGrammar = "shell"
	// TreeSitterShellGrammar identifies the pinned Bash grammar implementation.
	TreeSitterShellGrammar = "tree-sitter-bash@0.25.1"
)

type wrappedLanguageConfig struct {
	language         string
	rootKind         string
	expressionPrefix string
	expressionSuffix string
	statementPrefix  string
	statementSuffix  string
	statementBlocks  map[string]bool
	declarations     map[string]bool
	memberPrefix     string
	memberSuffix     string
	memberBlocks     map[string]bool
	shellLike        bool
}

func compileCTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(cLanguageConfig(false), decoded, maxDepth)
}

func compileCPPTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(cLanguageConfig(true), decoded, maxDepth)
}

func cLanguageConfig(cpp bool) wrappedLanguageConfig {
	language := "c"
	declarations := stringSet("declaration", "function_definition", "linkage_specification", "preproc_def", "preproc_function_def", "struct_specifier", "type_definition")
	if cpp {
		language = "cpp"
		declarations = stringSet("alias_declaration", "class_specifier", "concept_definition", "declaration", "enum_specifier", "function_definition", "linkage_specification", "namespace_definition", "template_declaration", "type_definition")
	}
	return wrappedLanguageConfig{
		language: language, rootKind: "translation_unit",
		expressionPrefix: "void __grit_func(){ (void)(", expressionSuffix: "); }\n",
		statementPrefix: "void __grit_func(){\n", statementSuffix: "\n}\n",
		statementBlocks: stringSet("compound_statement"), declarations: declarations,
	}
}

func compileCSharpTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(cSharpLanguageConfig(), decoded, maxDepth)
}

func cSharpLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "csharp", rootKind: "compilation_unit",
		expressionPrefix: "class __G { object F(){ return ", expressionSuffix: "; } }\n",
		statementPrefix: "class __G { void F(){\n", statementSuffix: "\n} }\n",
		statementBlocks: stringSet("block"),
		declarations:    stringSet("class_declaration", "delegate_declaration", "enum_declaration", "file_scoped_namespace_declaration", "global_statement", "interface_declaration", "namespace_declaration", "record_declaration", "struct_declaration", "using_directive"),
		memberPrefix:    "class __G {\n", memberSuffix: "\n}\n", memberBlocks: stringSet("declaration_list"),
	}
}

func compileJavaTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(javaLanguageConfig(), decoded, maxDepth)
}

func javaLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "java", rootKind: "program",
		expressionPrefix: "class __G { Object f(){ return ", expressionSuffix: "; } }\n",
		statementPrefix: "class __G { void f(){\n", statementSuffix: "\n} }\n",
		statementBlocks: stringSet("block"),
		declarations:    stringSet("class_declaration", "enum_declaration", "import_declaration", "interface_declaration", "module_declaration", "package_declaration", "record_declaration"),
		memberPrefix:    "class __G {\n", memberSuffix: "\n}\n", memberBlocks: stringSet("class_body"),
	}
}

func compileKotlinTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(kotlinLanguageConfig(), decoded, maxDepth)
}

func kotlinLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "kotlin", rootKind: "source_file",
		expressionPrefix: "val __grit_value = ", expressionSuffix: "\n",
		statementPrefix: "fun __grit_func(){\n", statementSuffix: "\n}\n",
		statementBlocks: stringSet("function_body", "statements"),
		declarations:    stringSet("class_declaration", "companion_object", "function_declaration", "import_header", "object_declaration", "package_header", "property_declaration", "type_alias"),
	}
}
func phpLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "php", rootKind: "program",
		expressionPrefix: "<?php $__grit_value = ", expressionSuffix: ";\n",
		statementPrefix: "<?php function __grit_func(){\n", statementSuffix: "\n}\n",
		statementBlocks: stringSet("compound_statement"),
		declarations:    stringSet("namespace_definition", "namespace_use_declaration", "function_definition", "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration"),
		memberPrefix:    "<?php class __G {\n", memberSuffix: "\n}\n", memberBlocks: stringSet("declaration_list"),
	}
}

func compilePHPTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(phpLanguageConfig(), decoded, maxDepth)
}

func compileDartTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	roles := make([]placeholderRole, len(decoded.placeholders))
	assignments := [][]placeholderRole{roles}
	if wholeSnippetPlaceholder(decoded) {
		statementRoles := append([]placeholderRole(nil), roles...)
		statementRoles[0] = roleDartStatement
		declarationRoles := append([]placeholderRole(nil), roles...)
		declarationRoles[0] = roleDartDeclaration
		assignments = append(assignments, statementRoles, declarationRoles)
	}
	return compileWrappedLanguageTemplatesWithRoles(dartLanguageConfig(), decoded, maxDepth, assignments)
}

func dartLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "dart", rootKind: "source_file",
		expressionPrefix: "void __grit_func(){ final __grit_value = ", expressionSuffix: "; }\n",
		statementPrefix: "void __grit_func(){\n", statementSuffix: "\n}\n",
		statementBlocks: stringSet("block"),
		declarations:    stringSet("class_declaration", "enum_declaration", "extension_declaration", "extension_type_declaration", "function_declaration", "getter_declaration", "import_or_export", "mixin_declaration", "part_directive", "setter_declaration", "top_level_variable_declaration", "type_alias"),
		memberPrefix:    "class __G {\n", memberSuffix: "\n}\n", memberBlocks: stringSet("class_body"),
	}
}

func compileSwiftTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(swiftLanguageConfig(), decoded, maxDepth)
}

func swiftLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "swift", rootKind: "source_file",
		expressionPrefix: "func __grit_func() { let __grit_value = ", expressionSuffix: "\n}\n",
		statementPrefix: "func __grit_func() {\n", statementSuffix: "\n}\n",
		statementBlocks: stringSet("statements", "function_body"),
		declarations:    stringSet("class_declaration", "protocol_declaration", "function_declaration", "import_declaration", "init_declaration", "property_declaration", "typealias_declaration"),
		memberPrefix:    "struct __G {\n", memberSuffix: "\n}\n", memberBlocks: stringSet("class_body"),
	}
}
func compileRustTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(rustLanguageConfig(), decoded, maxDepth)
}

func rustLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "rust", rootKind: "source_file",
		expressionPrefix: "fn __grit_func(){ let __grit_value = ", expressionSuffix: "; }\n",
		statementPrefix: "fn __grit_func(){\n", statementSuffix: "\n}\n",
		statementBlocks: stringSet("block"),
		declarations:    stringSet("const_item", "enum_item", "extern_crate_declaration", "foreign_mod_item", "function_item", "impl_item", "macro_definition", "mod_item", "static_item", "struct_item", "trait_item", "type_item", "union_item", "use_declaration"),
	}
}

func compileShellTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(shellLanguageConfig(), decoded, maxDepth)
}

func shellLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{language: "shell", rootKind: "program", declarations: stringSet("function_definition", "variable_assignment"), shellLike: true}
}

func compileWrappedLanguageTemplates(config wrappedLanguageConfig, decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	roles := make([]placeholderRole, len(decoded.placeholders))
	return compileWrappedLanguageTemplatesWithRoles(config, decoded, maxDepth, [][]placeholderRole{roles})
}

func compileWrappedLanguageTemplatesWithRoles(config wrappedLanguageConfig, decoded decodedSnippet, maxDepth int, assignments [][]placeholderRole) ([]Template, string, error) {
	attempts := wrappedSnippetAttempts(config)
	var templates []Template
	for _, attempt := range attempts {
		// Without an opening tag, PHP parses the entire snippet as inline HTML.
		// Such a file template would match arbitrary HTML rather than PHP syntax.
		if config.language == "php" && attempt.context == SnippetContextFile && !strings.Contains(decoded.text, "<?") {
			continue
		}
		candidates, tooDeep := parseInferredTemplates(config.language, decoded, attempt, assignments, maxDepth)
		if tooDeep {
			return nil, "LIMIT_PARSE_DEPTH", fmt.Errorf("%s template exceeds effective depth limit", config.language)
		}
		templates = append(templates, candidates...)
	}
	templates = dedupeTemplates(templates)
	if len(templates) == 0 {
		return nil, "PATTERN_INVALID_SNIPPET", fmt.Errorf("snippet is not valid %s in any supported context", config.language)
	}
	return templates, "", nil
}

func wrappedSnippetAttempts(config wrappedLanguageConfig) []snippetAttempt {
	var attempts []snippetAttempt
	if config.shellLike {
		attempts = append(attempts,
			snippetAttempt{SnippetContextStatement, "", "\n", selectExactSnippetNode},
			snippetAttempt{SnippetContextStatementList, "", "\n", selectRootSequence(config.rootKind, "statement_sequence")},
		)
	} else {
		attempts = append(attempts,
			snippetAttempt{SnippetContextExpression, config.expressionPrefix, config.expressionSuffix, selectExactSnippetNode},
			snippetAttempt{SnippetContextStatement, config.statementPrefix, config.statementSuffix, selectExactSnippetNode},
			snippetAttempt{SnippetContextStatementList, config.statementPrefix, config.statementSuffix, selectWrappedSequence(config.statementBlocks, "statement_sequence")},
		)
	}
	declarationPrefix := ""
	if config.language == "php" {
		declarationPrefix = "<?php\n"
	}
	attempts = append(attempts,
		snippetAttempt{SnippetContextDeclaration, declarationPrefix, "\n", selectExactDeclaration(config.declarations)},
		snippetAttempt{SnippetContextDeclarationList, declarationPrefix, "\n", selectRootSequence(config.rootKind, "declaration_sequence")},
	)
	if config.memberPrefix != "" {
		attempts = append(attempts,
			snippetAttempt{SnippetContextDeclaration, config.memberPrefix, config.memberSuffix, selectExactSnippetNode},
			snippetAttempt{SnippetContextDeclarationList, config.memberPrefix, config.memberSuffix, selectWrappedSequence(config.memberBlocks, "list_sequence")},
		)
	}
	attempts = append(attempts, snippetAttempt{SnippetContextFile, "", "", selectWrappedFile(config.rootKind)})
	return attempts
}

func selectExactSnippetNode(root parser.Node, start, end int) (selectedRoot, bool) {
	selected, depth := parser.Node{}, -1
	type item struct {
		node  parser.Node
		depth int
	}
	stack := []item{{node: root}}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rng := current.node.Range()
		if current.node.IsNamed() && rng.StartByte == start && rng.EndByte == end && current.depth > depth {
			selected, depth = current.node, current.depth
		}
		for _, child := range current.node.Children() {
			stack = append(stack, item{node: child, depth: current.depth + 1})
		}
	}
	return selectedRoot{node: selected}, selected.Valid()
}

func selectExactDeclaration(declarations map[string]bool) func(parser.Node, int, int) (selectedRoot, bool) {
	return func(root parser.Node, start, end int) (selectedRoot, bool) {
		selected, ok := selectExactSnippetNode(root, start, end)
		return selected, ok && declarations[selected.node.Kind()]
	}
}

func selectWrappedSequence(blockKinds map[string]bool, sequenceKind string) func(parser.Node, int, int) (selectedRoot, bool) {
	return func(root parser.Node, start, end int) (selectedRoot, bool) {
		for _, node := range namedDescendants(root) {
			if !blockKinds[node.Kind()] || !sequenceChildrenCover(node, start, end) {
				continue
			}
			return selectedRoot{node: node, sequenceKind: sequenceKind}, true
		}
		return selectedRoot{}, false
	}
}

func selectRootSequence(rootKind, sequenceKind string) func(parser.Node, int, int) (selectedRoot, bool) {
	return func(root parser.Node, start, end int) (selectedRoot, bool) {
		if root.Kind() != rootKind || !sequenceChildrenCover(root, start, end) {
			return selectedRoot{}, false
		}
		return selectedRoot{node: root, sequenceKind: sequenceKind}, true
	}
}

func selectWrappedFile(rootKind string) func(parser.Node, int, int) (selectedRoot, bool) {
	return func(root parser.Node, start, end int) (selectedRoot, bool) {
		return selectedRoot{node: root}, root.Kind() == rootKind && start == 0 && rangeWithin(root, start, end)
	}
}

func sequenceChildrenCover(parent parser.Node, start, end int) bool {
	children := directNamed(parent)
	inside := 0
	for _, child := range children {
		rng := child.Range()
		if rng.StartByte >= start && rng.EndByte <= end {
			inside++
		}
	}
	return inside >= 2
}

func namedDescendants(root parser.Node) []parser.Node {
	var result []parser.Node
	stack := []parser.Node{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node.IsNamed() {
			result = append(result, node)
		}
		children := node.Children()
		for index := len(children) - 1; index >= 0; index-- {
			stack = append(stack, children[index])
		}
	}
	return result
}

func wrappedRootCategoryAccepts(language string, declarations map[string]bool, context SnippetContext, kind string) bool {
	switch context {
	case SnippetContextExpression:
		if language == "dart" {
			return dartExpressionRootCategory(kind)
		}
		if language == "swift" {
			return swiftExpressionRootCategory(kind)
		}
		return grammarSubtypeAny(language, kind, "expression", "_expression", "primary_expression")
	case SnippetContextStatement, SnippetContextStatementList:
		return language == "swift" && swiftExpressionRootCategory(kind) || grammarSubtypeAny(language, kind, "statement", "_statement", "simple_statement", "_simple_statement") || declarations[kind]
	case SnippetContextDeclaration, SnippetContextDeclarationList:
		return declarations[kind]
	case SnippetContextFile:
		return kind == wrappedLanguageByID(language).rootKind
	default:
		return false
	}
}

// Swift's pinned grammar declares no expression subtype membership. Limit
// Swift snippet roots to named expression and identifier kinds in that grammar.
func swiftExpressionRootCategory(kind string) bool {
	return kind == "simple_identifier" || kind == "identifier" || kind == "call_expression" || strings.HasSuffix(kind, "_expression") && parser.NewParser().GetGrammar("swift").NodeKind(kind)
}

// Dart's pinned grammar has no declared expression supertype in node-types.json.
// Accept grammar-backed expression nodes and literal/instantiation subtypes only.
func dartExpressionRootCategory(kind string) bool {
	return kind == "identifier" || kind == "pattern_assignment" || strings.HasSuffix(kind, "_expression") && parser.NewParser().GetGrammar("dart").NodeKind(kind) || grammarSubtypeAny("dart", kind, "_literal", "_instantiation")
}

func grammarSubtypeAny(language, kind string, supertypes ...string) bool {
	for _, supertype := range supertypes {
		if parser.NewParser().GetGrammar(language).Subtype(supertype, kind) {
			return true
		}
	}
	return false
}

func wrappedLanguageByID(language string) wrappedLanguageConfig {
	switch language {
	case "c":
		return cLanguageConfig(false)
	case "cpp":
		return cLanguageConfig(true)
	case "csharp":
		return cSharpLanguageConfig()
	case "dart":
		return dartLanguageConfig()
	case "java":
		return javaLanguageConfig()
	case "kotlin":
		return kotlinLanguageConfig()
	case "rust":
		return rustLanguageConfig()
	case "shell":
		return shellLanguageConfig()
	case "swift":
		return swiftLanguageConfig()
	case "php":
		return phpLanguageConfig()
	default:
		return wrappedLanguageConfig{}
	}
}

func stringSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
