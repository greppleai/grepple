package rulespec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestNormalizeStructuralRuleCompilesAndPreservesCanonicalSource(t *testing.T) {
	request := &api.GritRequest{
		Query: "language go\n`target($x)`", Compatibility: api.GritCompatibilityV1,
		Repositories: []string{"acme/*"}, Globs: []string{"**/*.go"},
	}
	rule, err := Normalize(api.Rule{ID: "calls", Engine: api.RuleEngineGritQL, Structural: request})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Mode != api.RuleModeCount || Engine(rule) != api.RuleEngineGritQL || rule.Structural.Query != request.Query {
		t.Fatalf("rule=%#v", rule)
	}
	request.Repositories[0] = "changed"
	if rule.Structural.Repositories[0] != "acme/*" {
		t.Fatal("normalized rule retained caller-owned scope storage")
	}
}
func TestNormalizeTypeScriptStructuralRule(t *testing.T) {
	request := &api.GritRequest{
		Query: "language typescript\n`target($value)`", Compatibility: api.GritMultilingualCompatibilityV1, Globs: []string{"**/*.ts"},
	}
	rule, err := Normalize(api.Rule{ID: "typescript-calls", Engine: api.RuleEngineGritQL, Structural: request})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Structural.Compatibility != api.GritMultilingualCompatibilityV1 || rule.Structural.Query != request.Query {
		t.Fatalf("rule=%#v", rule)
	}
}

func TestNormalizeStructuralRuleRejectsInvalidDefinitions(t *testing.T) {
	query := func(source string) *api.GritRequest {
		return &api.GritRequest{Query: source, Compatibility: api.GritCompatibilityV1}
	}
	cases := []api.Rule{
		{ID: "missing", Engine: api.RuleEngineGritQL},
		{ID: "invalid", Engine: api.RuleEngineGritQL, Structural: query("language go\n`unterminated")},
		{ID: "wrong", Engine: api.RuleEngineGritQL, Structural: &api.GritRequest{Query: "language go\n`x`", Compatibility: "other"}},
		{ID: "mixed", Engine: api.RuleEngineGritQL, Request: api.SearchRequest{Globs: []string{"*.go"}}, Structural: query("language go\n`x`")},
		{ID: "glob", Engine: api.RuleEngineGritQL, Structural: &api.GritRequest{Query: "language go\n`x`", Compatibility: api.GritCompatibilityV1, Globs: []string{"[bad"}}},
		{ID: "paged", Engine: api.RuleEngineGritQL, Structural: func() *api.GritRequest {
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
	rule, err := Normalize(api.Rule{ID: "legacy", Engine: api.RuleEngineText, Request: api.SearchRequest{Query: &query}})
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
