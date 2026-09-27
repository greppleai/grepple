package parser

import "testing"

func TestGrammarNodeKindValidatesPinnedLanguages(t *testing.T) {
	for _, language := range SupportedLanguages() {
		if !GrammarNodeKind(language.ID, "identifier") && !GrammarNodeKind(language.ID, "source_file") && !GrammarNodeKind(language.ID, "program") {
			t.Fatalf("%s: no known named grammar node", language.ID)
		}
		if GrammarNodeKind(language.ID, "not_a_syntax_node_kind") {
			t.Fatalf("%s: accepted unknown node", language.ID)
		}
	}
	if GrammarNodeKind("missing-language", "source_file") {
		t.Fatal("unknown language accepted")
	}
	if !GrammarFieldName("go", "name") || GrammarFieldName("go", "not_a_go_grammar_field") || GrammarFieldName("missing-language", "name") {
		t.Fatal("pinned grammar field check failed")
	}
}
