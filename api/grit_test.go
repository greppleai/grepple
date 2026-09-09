package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestGritRequestJSONPreservesRequiredAndOptionalFields(t *testing.T) {
	findings := 7
	request := GritRequest{
		Query:         "language go\n`target($x)`",
		Compatibility: GritCompatibilityV1,
		Globs:         []string{"**/*.go"},
		Repositories:  []string{"acme/repo"},
		Limits:        &GritLimits{Findings: &findings},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"query":"language go\n` + "`target($x)`" + `","compatibility":"gritql-go-v1","globs":["**/*.go"],"repositories":["acme/repo"],"limits":{"findings":7}}`
	if string(encoded) != want {
		t.Fatalf("request JSON=%s want %s", encoded, want)
	}

	zero, err := json.Marshal(GritRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if string(zero) != `{"query":"","compatibility":""}` {
		t.Fatalf("zero request JSON=%s", zero)
	}
}

func TestGritResponseJSONUsesStableShapesAndNullableDiagnosticContext(t *testing.T) {
	response := GritResponse{
		Metadata: GritMetadata{Compatibility: GritCompatibilityV1, GoGrammar: "go1.25"},
		Findings: []GritFinding{{
			Repo: "acme/repo", Path: "a.go", Language: "go", Text: "target(x)", PatternID: "rule", Message: "found",
			Range:    GritRange{StartByte: 10, EndByte: 19, Start: GritPosition{Line: 2, Column: 1}, End: GritPosition{Line: 2, Column: 10}},
			Bindings: []GritBinding{{Name: "x", Kind: GritBindingNode, Range: GritRange{StartByte: 17, EndByte: 18}}},
		}},
		Diagnostics: []GritDiagnostic{{Code: "LIMIT_FINDINGS", Class: "resource", Severity: "error", Message: "rule finding limit reached"}},
		Truncations: []GritTruncation{},
		ShardErrors: []string{},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"findings", "diagnostics", "truncations", "shardErrors"} {
		if value, exists := decoded[field]; !exists || reflect.ValueOf(value).Kind() != reflect.Slice {
			t.Fatalf("%s is not a non-null array in %s", field, encoded)
		}
	}
	diagnostics := decoded["diagnostics"].([]any)
	diagnostic := diagnostics[0].(map[string]any)
	if diagnostic["patternId"] != nil || diagnostic["path"] != nil || diagnostic["range"] != nil {
		t.Fatalf("diagnostic nullable fields=%v", diagnostic)
	}
}

func TestGritRequestRejectsMalformedTypedJSON(t *testing.T) {
	var request GritRequest
	if err := json.Unmarshal([]byte(`{"query":"language go\n`+"`x`"+`","compatibility":"gritql-go-v1","skip":"one"}`), &request); err == nil {
		t.Fatal("malformed typed request decoded without error")
	}
}

func TestExistingSearchRequestJSONRemainsUnchanged(t *testing.T) {
	query := "x"
	regex := true
	encoded, err := json.Marshal(SearchRequest{Query: &query, Regex: &regex, Globs: []string{"*.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"query":"x","globs":["*.go"],"regex":true}` {
		t.Fatalf("legacy JSON changed: %s", encoded)
	}
	legacyResponse, err := json.Marshal(SearchResponse{})
	if err != nil {
		t.Fatal(err)
	}
	if string(legacyResponse) != `{"results":null}` {
		t.Fatalf("legacy response JSON changed: %s", legacyResponse)
	}
}

func TestGritWireHardLimitsAreClosed(t *testing.T) {
	if MaxGritRequestBodyBytes <= MaxGritQueryBytes || MaxGritQueryBytes != 1<<20 {
		t.Fatalf("body/query limits=%d/%d", MaxGritRequestBodyBytes, MaxGritQueryBytes)
	}
	if MaxGritGlobs <= 0 || MaxGritGlobBytes <= 0 || MaxGritRepositories <= 0 || MaxGritPatternIDBytes <= 0 || MaxGritMessageBytes <= 0 {
		t.Fatal("one or more structural wire limits are not positive")
	}
}

func TestRuleJSONKeepsLegacyTextShapeAndAddsStructuralDefinition(t *testing.T) {
	query := "x"
	legacy, err := json.Marshal(Rule{ID: "old", Mode: RuleModeCount, Request: SearchRequest{Query: &query}})
	if err != nil {
		t.Fatal(err)
	}
	if string(legacy) != `{"id":"old","mode":"count","request":{"query":"x"}}` {
		t.Fatalf("legacy rule JSON changed: %s", legacy)
	}
	structural, err := json.Marshal(Rule{
		ID: "calls", Mode: RuleModeCount, Engine: RuleEngineGritQL,
		Structural: &GritRequest{Query: "language go\n`x`", Compatibility: GritCompatibilityV1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(structural) != `{"id":"calls","mode":"count","engine":"gritql","request":{},"structural":{"query":"language go\n`+"`x`"+`","compatibility":"gritql-go-v1"}}` {
		t.Fatalf("structural rule JSON=%s", structural)
	}
}
