package parser

import "testing"

func TestGrammarNodeKindValidatesPinnedLanguages(t *testing.T) {
	service := NewParser()
	for _, language := range service.SupportedLanguages() {
		grammar := service.GetGrammar(language.ID)
		if !grammar.NodeKind("identifier") && !grammar.NodeKind("source_file") && !grammar.NodeKind("program") && !grammar.NodeKind("document") {
			t.Fatalf("%s: no known named grammar node", language.ID)
		}
		if grammar.NodeKind("not_a_syntax_node_kind") {
			t.Fatalf("%s: accepted unknown node", language.ID)
		}
	}
	if service.GetGrammar("missing-language").NodeKind("source_file") {
		t.Fatal("unknown language accepted")
	}
	grammar := service.GetGrammar("go")
	if !grammar.FieldName("name") || grammar.FieldName("not_a_go_grammar_field") || service.GetGrammar("missing-language").FieldName("name") {
		t.Fatal("pinned grammar field check failed")
	}
}

func TestParserGetGrammarReturnsUnknownAnswersForUnsupportedLanguage(t *testing.T) {
	grammar := NewParser().GetGrammar("missing-language")
	if grammar.NodeKind("source_file") || grammar.FieldName("name") || grammar.TokenKind("&&") || grammar.Subtype("_type", "identifier") || grammar.FieldCardinality("node", "field") != GrammarCardinalityUnknown || grammar.ChildrenCardinality("node") != GrammarCardinalityUnknown {
		t.Fatalf("unknown language produced grammar facts: %#v", grammar)
	}
}
