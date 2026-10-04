package gritql

import (
	"fmt"
	"github.com/greppleai/grepple/internal/parser"
)

// SvelteGrammar identifies the Svelte markup and template syntax contract.
const SvelteGrammar = "svelte"

// TreeSitterSvelteGrammar identifies the pinned Svelte grammar implementation.
const TreeSitterSvelteGrammar = "tree-sitter-svelte@1.0.2+javascript@0.25.0+typescript@0.23.2+css@0.25.0"

func compileSvelteTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	var templates []Template
	for _, config := range svelteSnippetConfigs() {
		candidates, code, err := compileWrappedLanguageTemplates(config, decoded, maxDepth)
		if err != nil && code != "PATTERN_INVALID_SNIPPET" {
			return nil, code, err
		}
		templates = append(templates, candidates...)
	}
	templates = dedupeTemplates(templates)
	if len(templates) == 0 {
		return nil, "PATTERN_INVALID_SNIPPET", fmt.Errorf("snippet is not valid Svelte markup, script or style syntax")
	}
	return templates, "", nil
}

func svelteLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "svelte", rootKind: "document",
		statementPrefix: "<div>", statementSuffix: "</div>",
		statementBlocks: stringSet("element"),
		declarations: stringSet("element", "self_closing_tag", "script_element", "style_element",
			"if_statement", "each_statement", "await_statement", "key_statement", "snippet_statement",
			"expression", "html_tag", "const_tag", "debug_tag", "render_tag"),
	}
}

func svelteTargetLanguageAdapter() targetLanguageAdapter {
	adapter := wrappedTargetLanguageAdapter("svelte", SvelteGrammar, TreeSitterSvelteGrammar, compileSvelteTemplates)
	adapter.placeholderCardinality = sveltePlaceholderCardinality
	adapter.rootCategory = func(context SnippetContext, kind string) bool {
		return wrappedRootCategoryAccepts("svelte", svelteLanguageConfig().declarations, context, kind) ||
			typeScriptRootCategoryAccepts(context, kind) || javaScriptRootCategoryAccepts(context, kind) ||
			wrappedRootCategoryAccepts("css", cssLanguageConfig().declarations, context, kind)
	}
	return adapter
}

func sveltePlaceholderCardinality(node, parent parser.Node) (SlotCardinality, bool) {
	switch node.Kind() {
	case "tag_name", "snippet_name", "attribute_name", "svelte_raw_text", "raw_text":
		return SlotOne, true
	case "attribute", "text":
		if parent.Valid() && repeatedGrammarPosition("svelte", parent, node) {
			return SlotMany, true
		}
		return SlotOne, true
	default:
		return cssPlaceholderCardinality(node, parent)
	}
}

func svelteSnippetConfigs() []wrappedLanguageConfig {
	configs := []wrappedLanguageConfig{svelteLanguageConfig()}
	for _, prefix := range []string{"<script>", "<script lang=\"ts\">"} {
		configs = append(configs, wrappedLanguageConfig{
			language: "svelte", rootKind: "document",
			expressionPrefix: prefix + "const __grit_expr = (", expressionSuffix: ");</script>",
			statementPrefix: prefix, statementSuffix: "</script>",
			statementBlocks: stringSet("program", "statement_block"),
			memberPrefix:    prefix, memberSuffix: "</script>", memberBlocks: stringSet("program"),
			declarations: stringSet("function_declaration", "generator_function_declaration", "class_declaration", "lexical_declaration", "variable_declaration", "interface_declaration", "type_alias_declaration", "enum_declaration", "import_statement", "export_statement"),
		})
	}
	css := cssLanguageConfig()
	css.language, css.rootKind = "svelte", "document"
	css.expressionPrefix, css.expressionSuffix = "<style>"+css.expressionPrefix, css.expressionSuffix+"</style>"
	css.statementPrefix, css.statementSuffix = "<style>"+css.statementPrefix, css.statementSuffix+"</style>"
	css.memberPrefix, css.memberSuffix, css.memberBlocks = "<style>", "</style>", stringSet("stylesheet")
	return append(configs, css)
}
