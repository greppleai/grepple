package parser

import sitter "github.com/tree-sitter/go-tree-sitter"

type stringSet map[string]struct{}

func newStringSet(values ...string) stringSet {
	set := make(stringSet, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func (set stringSet) contains(value string) bool {
	_, ok := set[value]
	return ok
}

type structureRules struct {
	structuralTypes       stringSet
	contextTypes          stringSet
	containerTypes        stringSet
	classDeclarationTypes stringSet
	classBodyTypes        stringSet
	exportTypes           stringSet
	functionLikeTypes     stringSet
	blockTypes            stringSet
	jsxElementTypes       stringSet
	nameFieldCandidates   stringSet
}

type languageAdapter interface {
	ID() string
	Grammar() *sitter.Language
	Rules() *structureRules
	Outline(root *sitter.Node, content string) []Symbol
}

var languageAdapters = buildLanguageAdapters(
	newGoLanguage(),
	newJavaLanguage(),
	newKotlinLanguage(),
	newJavaScriptLanguage(),
	newTypeScriptLanguage("typescript", false),
	newTypeScriptLanguage("tsx", true),
	newPythonLanguage(),
	newCSharpLanguage(),
	newCLanguage(),
	newCPPLanguage(),
	newRustLanguage(),
	newShellLanguage(),
)

func buildLanguageAdapters(adapters ...languageAdapter) map[string]languageAdapter {
	registry := make(map[string]languageAdapter, len(adapters))
	for _, adapter := range adapters {
		registry[adapter.ID()] = adapter
	}
	return registry
}

func adapterForLanguage(id string) languageAdapter {
	return languageAdapters[id]
}

func descendantName(node *sitter.Node, content string, candidates stringSet) string {
	if candidates.contains(node.Kind()) {
		return nodeText(node, content)
	}
	for _, child := range namedChildren(node) {
		if name := descendantName(child, content, candidates); name != "" {
			return name
		}
	}
	return ""
}
