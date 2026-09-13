package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestGraphJSONEmitsResolvedDeterministicGraph(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", "package sample\nfunc Run(){ helper() }\n")
	writeGraphSource(t, dir, "helper.go", "package sample\nfunc helper() {}\n")
	writeGraphSource(t, dir, "notes.txt", "ignored\n")
	first := captureStdout(t, func() {
		if err := Run([]string{"graph", "--json", "main.go", "helper.go"}); err != nil {
			t.Fatal(err)
		}
	})
	second := captureStdout(t, func() {
		if err := Run([]string{"graph", "--json", "helper.go", "main.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if first != second {
		t.Fatalf("graph output is not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}
	var output navigationGraphOutput
	if err := json.Unmarshal([]byte(first), &output); err != nil {
		t.Fatal(err)
	}
	if output.Schema != navigationGraphSchema || output.Files != 2 || len(output.Declarations) != 2 || len(output.Calls) != 1 {
		t.Fatalf("output=%#v", output)
	}
	call := output.Calls[0]
	if call.TargetID == "" || call.Confidence != "unique-terminal" || call.CallerID == "" {
		t.Fatalf("call=%#v", call)
	}
	for _, declaration := range output.Declarations {
		if filepath.IsAbs(declaration.Path) {
			t.Fatalf("absolute declaration path: %#v", declaration)
		}
	}
}

func TestGraphJSONReportsFileTruncation(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "a.go", "package sample\nfunc A() {}\n")
	writeGraphSource(t, dir, "b.py", "def b():\n    pass\n")
	outputText := captureStdout(t, func() {
		if err := Run([]string{"graph", "--json", "--max-files", "1"}); err != nil {
			t.Fatal(err)
		}
	})
	var output navigationGraphOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		t.Fatal(err)
	}
	want := &navigationGraphTruncation{Reason: "max_files", Limit: 1, Skipped: 1}
	if output.Files != 1 || !reflect.DeepEqual(output.Truncation, want) {
		t.Fatalf("output=%#v", output)
	}
}

func TestGraphRequiresJSONAndRejectsNegativeLimit(t *testing.T) {
	if err := Run([]string{"graph"}); err == nil {
		t.Fatal("expected graph without an output mode to fail")
	}
	if err := Run([]string{"graph", "--json", "--compact"}); err == nil {
		t.Fatal("expected conflicting graph output modes to fail")
	}
	if err := Run([]string{"graph", "--json", "--max-files", "-1"}); err == nil {
		t.Fatal("expected negative max-files to fail")
	}
}

func TestGraphCompactEmitsBoundedAgentFacingEdges(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", "package sample\nfunc Run(){ helper() }\nfunc helper() {}\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"graph", "--compact", "--max-output-bytes", "4096"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"graph grepple-navigation-graph-v1 files=1 declarations=2 calls=1", "D ", " go func Run main.go:2", "C ", " Run -> helper#", "[unique-terminal] main.go:2"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("compact graph missing %q:\n%s", expected, output)
		}
	}
	truncated := captureStdout(t, func() {
		if err := Run([]string{"graph", "--compact", "--max-output-bytes", "220"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(truncated) > 220 || !strings.Contains(truncated, "truncated") {
		t.Fatalf("bounded output len=%d:\n%s", len(truncated), truncated)
	}
}

func TestNavigationGraphJSONUsesStableFieldNames(t *testing.T) {
	content, err := json.Marshal(parser.NavigationGraph{Calls: []parser.NavigationCall{{ID: "call", CallerID: "caller", Confidence: "candidate", CandidateTargetIDs: []string{"one", "two"}}}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatal(err)
	}
	calls, ok := decoded["calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("JSON=%s", content)
	}
	call := calls[0].(map[string]any)
	if call["callerId"] != "caller" || call["confidence"] != "candidate" {
		t.Fatalf("call JSON=%v", call)
	}
	if _, legacy := call["CallerID"]; legacy {
		t.Fatalf("legacy Go field name leaked: %s", content)
	}
}

func TestGraphCalleesQueryTraversesByDepth(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "flow.go", "package sample\nfunc Root(){ Middle() }\nfunc Middle(){ Leaf() }\nfunc Leaf() {}\n")
	outputText := captureStdout(t, func() {
		if err := Run([]string{"graph", "callees", "--symbol", "Root", "--depth", "2", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var output navigationGraphOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		t.Fatal(err)
	}
	if output.Query == nil || output.Query.Direction != "callees" || output.Query.Depth != 2 || len(output.Query.RootIDs) != 1 {
		t.Fatalf("query=%#v", output.Query)
	}
	if names := graphOutputDeclarationNames(output); !reflect.DeepEqual(names, []string{"Root", "Middle", "Leaf"}) {
		t.Fatalf("declarations=%v", names)
	}
	if len(output.Calls) != 2 {
		t.Fatalf("calls=%#v", output.Calls)
	}
}

func TestGraphCallersQuerySupportsAtAndCompactOutput(t *testing.T) {
	dir := chdirTemp(t)
	path := writeGraphSource(t, dir, "flow.go", "package sample\nfunc Root(){ Middle() }\nfunc Middle() {}\nfunc Consumer(){ Root() }\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"graph", "callers", "--at", path + ":3", "--depth", "2", "--compact"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"query callers depth=2 roots=", " go func Root ", " go func Middle ", " go func Consumer ", "Consumer -> Root#", "Root -> Middle#"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("callers output missing %q:\n%s", expected, output)
		}
	}
}

func TestGraphDependencyQueriesSupportScopeRoots(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "go.mod", "module example.com/project\n")
	writeGraphSource(t, dir, "app/app.go", "package app\nimport \"example.com/project/helper\"\nfunc Run(){ helper.Work() }\n")
	writeGraphSource(t, dir, "helper/helper.go", "package helper\nfunc Work() {}\n")
	for _, test := range []struct {
		direction, rootPath string
	}{
		{"dependencies", "app"},
		{"dependents", "helper"},
	} {
		output := captureStdout(t, func() {
			if err := Run([]string{"graph", test.direction, "--root-path", test.rootPath, "--depth", "1", "--compact", "."}); err != nil {
				t.Fatal(err)
			}
		})
		for _, expected := range []string{"query " + test.direction + " depth=1 roots=", " go func Run ", " go func Work ", "Run -> Work#"} {
			if !strings.Contains(output, expected) {
				t.Fatalf("%s output missing %q:\n%s", test.direction, expected, output)
			}
		}
	}
}

func TestGraphImpactQueryTraversesBothDirections(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "flow.go", "package sample\nfunc Root(){ Middle() }\nfunc Middle(){ Leaf() }\nfunc Leaf() {}\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"graph", "impact", "--symbol", "Middle", "--depth", "1", "--compact"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"query impact depth=1 roots=", " go func Root ", " go func Middle ", " go func Leaf ", "Root -> Middle#", "Middle -> Leaf#"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("impact output missing %q:\n%s", expected, output)
		}
	}
}

func TestGraphQueryFiltersLanguageAndConfidenceBeforeRootSelection(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "flow.go", "package sample\nfunc Root(){ Middle() }\nfunc Middle() {}\n")
	writeGraphSource(t, dir, "flow.py", "def Root():\n    pass\n")
	outputText := captureStdout(t, func() {
		if err := Run([]string{"graph", "callees", "--symbol", "Root", "--language", "go", "--confidence", "unique-terminal", "--visibility", "public", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var output navigationGraphOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		t.Fatal(err)
	}
	if output.Query == nil || !reflect.DeepEqual(output.Query.Languages, []string{"go"}) || !reflect.DeepEqual(output.Query.Confidences, []string{"unique-terminal"}) || !reflect.DeepEqual(output.Query.Visibilities, []string{"public"}) {
		t.Fatalf("query filters=%#v", output.Query)
	}
	if names := graphOutputDeclarationNames(output); !reflect.DeepEqual(names, []string{"Root", "Middle"}) {
		t.Fatalf("declarations=%v", names)
	}
	if len(output.Calls) != 1 {
		t.Fatalf("calls=%#v", output.Calls)
	}
}

func TestGraphQueryRejectsMissingAndAmbiguousSelectors(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "a.go", "package sample\nfunc helper() {}\n")
	writeGraphSource(t, dir, "b.go", "package sample\nfunc helper() {}\n")
	for _, arguments := range [][]string{
		{"graph", "callers", "--compact"},
		{"graph", "callees", "--symbol", "helper", "--compact"},
		{"graph", "callers", "--symbol", "helper", "--at", "a.go:2", "--compact"},
		{"graph", "dependencies", "--package", "sample", "--module", "sample", "--compact"},
		{"graph", "dependents", "--root-path", "missing", "--compact"},
		{"graph", "callers", "--symbol", "helper", "--language", "text", "--compact"},
		{"graph", "callers", "--symbol", "helper", "--confidence", "likely", "--compact"},
		{"graph", "callers", "--symbol", "helper", "--visibility", "maybe", "--compact"},
		{"graph", "callers", "--symbol", "missing", "--compact"},
		{"graph", "callers", "--symbol", "helper", "--depth", "11", "--compact"},
	} {
		if err := Run(arguments); err == nil {
			t.Fatalf("arguments %v unexpectedly succeeded", arguments)
		}
	}
}

func graphOutputDeclarationNames(output navigationGraphOutput) []string {
	names := make([]string, 0, len(output.Declarations))
	for _, declaration := range output.Declarations {
		names = append(names, declaration.Name)
	}
	return names
}

func writeGraphSource(t *testing.T, root, path, content string) string {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return fullPath
}
