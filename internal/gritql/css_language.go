package gritql

import "github.com/greppleai/grepple/internal/parser"

// CSSGrammar identifies the native stylesheet syntax contract.
const CSSGrammar = "css"

// TreeSitterCSSGrammar identifies the pinned CSS grammar implementation.
const TreeSitterCSSGrammar = "tree-sitter-css@0.25.0"

func compileCSSTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileWrappedLanguageTemplates(cssLanguageConfig(), decoded, maxDepth)
}
func cssLanguageConfig() wrappedLanguageConfig {
	return wrappedLanguageConfig{
		language: "css", rootKind: "stylesheet",
		expressionPrefix: ".__grit_context { property: ", expressionSuffix: "; }",
		statementPrefix: ".__grit_context {", statementSuffix: "}",
		statementBlocks: stringSet("block"),
		declarations:    stringSet("rule_set", "declaration", "at_rule", "media_statement", "supports_statement", "scope_statement", "keyframes_statement", "import_statement", "charset_statement", "namespace_statement"),
	}
}
func cssTargetLanguageAdapter() targetLanguageAdapter {
	adapter := wrappedTargetLanguageAdapter("css", CSSGrammar, TreeSitterCSSGrammar, compileCSSTemplates)
	adapter.placeholderCardinality = cssPlaceholderCardinality
	return adapter
}
func cssPlaceholderCardinality(node, parent parser.Node) (SlotCardinality, bool) {
	switch node.Kind() {
	case "tag_name", "property_name", "class_name", "id_name", "keyframes_name", "function_name":
		return SlotOne, true
	case "plain_value":
		if parent.Kind() == "arguments" || parent.Kind() == "declaration" {
			return SlotMany, true
		}
		return SlotOne, true
	}
	return SlotOne, false
}
