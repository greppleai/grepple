package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRelatedOmissionCountsUseStableJSONFields(t *testing.T) {
	result := FileResult{
		Path: "caller.go", Language: "go", Matches: []ResultMatch{}, Segments: []ResultSegment{},
		OmittedRelatedCallers: 2, OmittedRelatedCallees: 1, OmittedRelatedTypes: 6,
		Related: []RelatedSymbol{{Name: "target", Role: "parameter", OmittedCallers: 4, OmittedCallees: 3, OmittedTypes: 5}},
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"omittedRelatedCallers":2`, `"omittedRelatedCallees":1`, `"omittedRelatedTypes":6`,
		`"role":"parameter"`, `"omittedCallers":4`, `"omittedCallees":3`, `"omittedTypes":5`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("JSON missing %s: %s", field, encoded)
		}
	}
}

func TestSearchRequestLineRangesUseStableJSONFields(t *testing.T) {
	encoded, err := json.Marshal(SearchRequest{LineRanges: true, EnclosingRanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"matchLineRanges":true,"enclosingLineRanges":true}` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
}

func TestRelatedDependencyIdentityUsesStableJSONFields(t *testing.T) {
	symbol := RelatedSymbol{
		Name: "Client.Send", Path: "client/client.go", Confidence: "dependency-resolved",
		External: &ExternalNavigationReference{ID: "reference", Language: "go", ImportPath: "example.com/acme/client/client", Package: "example.com/acme/client/client", Symbol: "Send", ConsumerPackage: "example.com/consumer", ReceiverType: "Client", Kind: "call", Module: "example.com/acme/client", Version: "v1.4.2", Integrity: "h1:checksum", Source: "https://proxy.example.test"},
		Artifact: &NavigationArtifactIdentity{Ecosystem: "go", Module: "example.com/acme/client", Version: "v1.4.2", Source: "https://proxy.example.test", Repository: "acme/client@tag~v1.4.2", Commit: "abc", Digest: "digest"},
	}
	encoded, err := json.Marshal(symbol)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"confidence":"dependency-resolved"`, `"importPath":"example.com/acme/client/client"`, `"consumerPackage":"example.com/consumer"`, `"module":"example.com/acme/client"`, `"version":"v1.4.2"`, `"integrity":"h1:checksum"`, `"source":"https://proxy.example.test"`, `"commit":"abc"`, `"digest":"digest"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("dependency JSON missing %s: %s", field, encoded)
		}
	}
}

func TestExternalDependencyCandidatesUseStableJSONFields(t *testing.T) {
	reference := ExternalNavigationReference{
		ID: "reference", Language: "java", ImportPath: "com.acme.Widget", Symbol: "Widget", Kind: "type",
		Candidates: []ExternalDependencyCandidate{{Ecosystem: "maven", Module: "com.acme:widgets", Version: "2.3.0"}},
	}
	encoded, err := json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"candidates"`, `"ecosystem":"maven"`, `"module":"com.acme:widgets"`, `"version":"2.3.0"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("candidate JSON missing %s: %s", field, encoded)
		}
	}
}

func TestResultMatchRangesAreAdditive(t *testing.T) {
	encoded, err := json.Marshal(ResultMatch{Line: 3, EndLine: 5, Text: "if ready {"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"line":3,"endLine":5,"text":"if ready {"}` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
	encoded, err = json.Marshal(ResultMatch{Line: 4, StartLine: 3, EndLine: 5, Text: "work()"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"line":4,"startLine":3,"endLine":5,"text":"work()"}` {
		t.Fatalf("unexpected enclosing JSON: %s", encoded)
	}
	encoded, err = json.Marshal(ResultMatch{Line: 7, Text: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "startLine") || strings.Contains(string(encoded), "endLine") {
		t.Fatalf("zero endLine should remain wire-compatible: %s", encoded)
	}
}

func TestSourceAnalysisUsesAdditiveStableJSONFields(t *testing.T) {
	response := SearchResponse{
		Results:        []FileResult{{Path: "broken.go", Language: "go", StructureStatus: "recovered", Matches: []ResultMatch{}, Segments: []ResultSegment{}}},
		SourceAnalysis: &SourceAnalysis{Returned: 1, Recovered: 1},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"structureStatus":"recovered"`, `"sourceAnalysis":{"returned":1`, `"recovered":1`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("JSON missing %s: %s", field, encoded)
		}
	}
}

func TestResultMetadataUsesStableCrossCommandFields(t *testing.T) {
	total := 4
	metadata := ResultMetadata{
		Scope:   ResultScope{Mode: "local", Paths: []string{"."}, Repositories: []string{}, Languages: []string{"go"}},
		Order:   "path",
		Page:    ResultPage{Skip: 1, Limit: 2, Returned: 2, Total: &total},
		Limits:  ResultLimits{MaxFiles: 10, MaxOutputBytes: 16384, JSONByteUncapped: true},
		Omitted: ResultOmissions{Files: 2}, Diagnostics: []ResultDiagnostic{{Code: "source-cap", Message: "capped"}},
		NextCommand: "grepple search --skip 3",
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"scope":{"mode":"local"`, `"order":"path"`, `"page":{"skip":1`, `"limits":{"maxFiles":10`, `"omitted":{"files":2`, `"diagnostics":[{"code":"source-cap"`, `"nextCommand":"grepple search --skip 3"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("JSON missing %s: %s", field, encoded)
		}
	}
}
