package rulespec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/greppleai/grepple/search"
)

func TestNormalizeStructuralRuleCompilesAndPreservesCanonicalSource(t *testing.T) {
	request := &StructuralRequest{
		Query: "language go\n`target($x)`", Compatibility: GritCompatibilityV1,
		Repositories: []string{"acme/*"}, Globs: []string{"**/*.go"},
	}
	rule, err := Normalize(Rule{ID: "calls", Engine: EngineGritQL, Structural: request})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Mode != ModeCount || Engine(rule) != EngineGritQL || rule.Structural.Query != request.Query {
		t.Fatalf("rule=%#v", rule)
	}
	request.Repositories[0] = "changed"
	if rule.Structural.Repositories[0] != "acme/*" {
		t.Fatal("normalized rule retained caller-owned scope storage")
	}
}
func TestNormalizeTypeScriptStructuralRule(t *testing.T) {
	request := &StructuralRequest{
		Query: "language typescript\n`target($value)`", Compatibility: GritCompatibilityV1, Globs: []string{"**/*.ts"},
	}
	rule, err := Normalize(Rule{ID: "typescript-calls", Engine: EngineGritQL, Structural: request})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Structural.Compatibility != GritCompatibilityV1 || rule.Structural.Query != request.Query {
		t.Fatalf("rule=%#v", rule)
	}
}

func TestNormalizeStructuralRuleRejectsInvalidDefinitions(t *testing.T) {
	query := func(source string) *StructuralRequest {
		return &StructuralRequest{Query: source, Compatibility: GritCompatibilityV1}
	}
	cases := []Rule{
		{ID: "missing", Engine: EngineGritQL},
		{ID: "invalid", Engine: EngineGritQL, Structural: query("language go\n`unterminated")},
		{ID: "wrong", Engine: EngineGritQL, Structural: &StructuralRequest{Query: "language go\n`x`", Compatibility: "other"}},
		{ID: "mixed", Engine: EngineGritQL, Request: search.Request{Globs: []string{"*.go"}}, Structural: query("language go\n`x`")},
		{ID: "glob", Engine: EngineGritQL, Structural: &StructuralRequest{Query: "language go\n`x`", Compatibility: GritCompatibilityV1, Globs: []string{"[bad"}}},
		{ID: "paged", Engine: EngineGritQL, Structural: func() *StructuralRequest {
			request := query("language go\n`x`")
			request.Limit = intPointer(1)
			return request
		}()},
	}
	for _, rule := range cases {
		if _, err := Normalize(rule); err == nil {
			t.Fatalf("expected rejection for %#v", rule)
		}
	}
}

func TestNormalizeLegacyTextRuleKeepsZeroEngineWireShape(t *testing.T) {
	query := "needle"
	rule, err := Normalize(Rule{ID: "legacy", Engine: EngineText, Request: search.Request{Query: &query}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Engine != "" || strings.Contains(string(encoded), `"engine"`) || strings.Contains(string(encoded), `"structural"`) {
		t.Fatalf("legacy wire shape changed: %s", encoded)
	}
}

func intPointer(value int) *int { return &value }
