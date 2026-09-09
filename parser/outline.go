package parser

import (
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// OutlineFile parses content according to the file's language and returns its
// structural outline. Unsupported languages yield an outline with no symbols
// (Markdown and JSON/YAML are handled with lightweight scanners rather than
// tree-sitter).
func OutlineFile(path, content string) FileOutline {
	return OutlineFileDepth(path, content, 0)
}

// OutlineFileDepth is OutlineFile with a nesting cap for the structured-data
// formats (JSON/YAML): maxDepth <= 0 means unlimited, maxDepth N shows at most N
// levels of keys. The cap is ignored for code and Markdown outlines.
func OutlineFileDepth(path, content string, maxDepth int) FileOutline {
	if isMarkdownPath(path) {
		return FileOutline{Path: path, Language: "markdown", Symbols: nonNil(outlineMarkdown(content))}
	}
	if lang := structuredLang(path); lang != "" {
		return FileOutline{Path: path, Language: lang, Symbols: nonNil(outlineStructured(content, maxDepth))}
	}
	lang := LanguageFor(path)
	out := FileOutline{Path: path, Language: lang, Symbols: []Symbol{}}
	adapter := adapterForLanguage(lang)
	if adapter == nil || adapter.Grammar() == nil {
		return out
	}
	tree, err := parseTree(adapter, content)
	if err != nil {
		return out
	}
	defer tree.Close()
	out.Symbols = nonNil(adapter.Outline(tree.RootNode(), content))
	return out
}

func nonNil(s []Symbol) []Symbol {
	if s == nil {
		return []Symbol{}
	}
	return s
}

func isMarkdownPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// symbolFrom builds a Symbol for a node, capturing its line bounds and a compact
// one-line signature.
func symbolFrom(kind, name string, node *sitter.Node, content string) Symbol {
	return Symbol{
		Kind:      kind,
		Name:      name,
		Signature: declSignature(node, content),
		Start:     nodeStart(node),
		End:       nodeEnd(node),
	}
}

// declSignature returns the declaration line of a node with its body stripped:
// everything up to the first "{" (or first newline for bodyless declarations),
// whitespace-collapsed onto a single line.
func declSignature(node *sitter.Node, content string) string {
	text := nodeText(node, content)
	if i := strings.IndexByte(text, '{'); i >= 0 {
		text = text[:i]
	} else if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.Join(strings.Fields(text), " ")
}
