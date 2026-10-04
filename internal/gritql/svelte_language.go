package gritql

import "github.com/greppleai/grepple/internal/parser"

// SvelteGrammar identifies the Svelte markup and template syntax contract.
const SvelteGrammar = "svelte"

// TreeSitterSvelteGrammar identifies the pinned Svelte grammar implementation.
const TreeSitterSvelteGrammar = "tree-sitter-svelte@1.0.2"

func compileSvelteTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(svelteLanguageConfig(), decoded, maxDepth)
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
		return SlotOne, false
	}
}
