package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

func TestGraphJSONEmitsResolvedDeterministicGraph(t *testing.T) {
	dir := chdirTemp(t)
	t.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(dir, ".grepple", "cache", "navigation"))
	writeGraphSource(t, dir, "main.go", "package sample\ntype Input struct{ Value string }\nfunc Run(input Input){ helper() }\n")
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
	if output.Schema != navigationGraphSchema || output.Files != 2 || len(output.Declarations) != 2 || len(output.TypeDeclarations) != 1 || len(output.TypeUsages) != 1 || len(output.Calls) != 1 {
		t.Fatalf("output=%#v", output)
	}
	assertGraphResolutionStats(t, output.Resolution)
	if output.Metadata == nil || !output.Metadata.Page.Complete || output.Metadata.Limits.MaxOutputBytes != defaultTextOutputBytes || len(output.Metadata.Scope.Paths) != 2 {
		t.Fatalf("graph metadata=%#v", output.Metadata)
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

func TestGraphResolutionSeparatesExpectedExternalFromCandidateConfidence(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.ts", "import { client } from \"remote-package\";\nexport function run(){ client.send(); }\n")
	outputText := captureStdout(t, func() {
		if err := Run([]string{"graph", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var output navigationGraphOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		t.Fatal(err)
	}
	stats := output.Resolution
	if stats.ExpectedExternal != 1 || stats.ExpectedExternalRate != 1 || stats.UnresolvedLocal != 0 || len(stats.Confidences) != 1 || stats.Confidences[0] != (search.NavigationResolutionCount{Name: "candidate", Count: 1}) {
		t.Fatalf("external resolution stats=%#v", stats)
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
	if output.Metadata == nil || output.Metadata.Omitted.Sources != 1 || !strings.Contains(output.Metadata.NextCommand, "--max-files 0 --json") {
		t.Fatalf("truncated graph metadata=%#v", output.Metadata)
	}
	human := captureStdout(t, func() {
		if err := Run([]string{"graph", "callees", "--symbol", "A", "--max-files", "1"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"! truncated max_files limit=1 skipped=1", "continue: grepple graph callees", "go func A() @ a.go:2"} {
		if !strings.Contains(human, expected) {
			t.Fatalf("incomplete human graph hides %q:\n%s", expected, human)
		}
	}
	if strings.Contains(human, "query callees depth=") || strings.Contains(human, "grepple-navigation-graph-v7") {
		t.Fatalf("truncation warning does not require redundant query headers:\n%s", human)
	}
}

func TestGraphDefaultsToHumanOutputAndRejectsNegativeLimit(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", "package sample\nfunc Run() {}\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"graph"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "main.go") || !strings.Contains(output, "func Run") {
		t.Fatalf("default graph output = %q", output)
	}
	if err := Run([]string{"graph", "--compact"}); err == nil {
		t.Fatal("obsolete --compact should be rejected")
	}
	if err := Run([]string{"graph", "--json", "--max-files", "-1"}); err == nil {
		t.Fatal("expected negative max-files to fail")
	}
}

func TestGraphCompactEmitsBoundedAgentFacingEdges(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", "package sample\nfunc Run(){ helper() }\nfunc helper() {}\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"graph", "--max-output-bytes", "4096"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"graph files=1 declarations=2 calls=1", "\nmain.go\n", "  2 func Run visibility=public", "    -> helper:3 call:2 [unique-terminal]", "  3 func helper visibility=non-public"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("compact graph missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "grepple-navigation-graph-v7") || strings.Contains(output, "rates=") {
		t.Fatalf("human graph repeats JSON metadata:\n%s", output)
	}
	truncated := captureStdout(t, func() {
		if err := Run([]string{"graph", "--max-output-bytes", "80"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(truncated) > 80 || !strings.Contains(truncated, "…") {
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
		if err := Run([]string{"graph", "callers", "--at", path + ":3", "--depth", "2"}); err != nil {
			t.Fatal(err)
		}
	})
	want := "go func Middle() @ flow.go:3\n" +
		"<- func Root() @ flow.go:2 call:2 [unique-terminal]\n" +
		"  <- func Consumer() @ flow.go:4 call:4 [unique-terminal]\n"
	if output != want {
		t.Fatalf("focused callers should not repeat query metadata:\n%s\nwant:\n%s", output, want)
	}
}

func TestGraphCallersAndCalleesSupportScopeRoots(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "go.mod", "module example.com/project\n")
	writeGraphSource(t, dir, "app/app.go", "package app\nimport \"example.com/project/helper\"\nfunc Run(){ helper.Work() }\n")
	writeGraphSource(t, dir, "helper/helper.go", "package helper\nfunc Work() {}\n")
	for _, test := range []struct{ direction, rootPath string }{
		{"callees", "app"},
		{"callers", "helper"},
	} {
		jsonText := captureStdout(t, func() {
			if err := Run([]string{"graph", test.direction, "--root-path", test.rootPath, "--depth", "1", "--json", "."}); err != nil {
				t.Fatal(err)
			}
		})
		var result navigationGraphOutput
		if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
			t.Fatal(err)
		}
		if result.Query == nil || result.Query.Direction != test.direction || len(result.Calls) != 1 {
			t.Fatalf("%s scope query=%+v", test.direction, result.Query)
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
		{"graph", "callers"},
		{"graph", "callees", "--symbol", "helper"},
		{"graph", "callers", "--symbol", "helper", "--at", "a.go:2"},
		{"graph", "callees", "--package", "sample", "--module", "sample"},
		{"graph", "callers", "--root-path", "missing"},
		{"graph", "callers", "--symbol", "helper", "--language", "text"},
		{"graph", "callers", "--symbol", "helper", "--confidence", "likely"},
		{"graph", "callers", "--symbol", "helper", "--visibility", "maybe"},
		{"graph", "callers", "--symbol", "missing"},
		{"graph", "callers", "--symbol", "helper", "--depth", "11"},
	} {
		if err := Run(arguments); err == nil {
			t.Fatalf("arguments %v unexpectedly succeeded", arguments)
		}
	}
}

func TestGraphDiffReportsSemanticChangesAndIgnoresLineShifts(t *testing.T) {
	root := t.TempDir()
	before, after := filepath.Join(root, "before"), filepath.Join(root, "after")
	writeGraphSource(t, before, "service.go", "package sample\nfunc Run() { helper() }\nfunc helper() {}\nfunc removed() {}\n")
	writeGraphSource(t, after, "service.go", "package sample\n\nfunc Run() { added() }\nfunc helper() {}\nfunc added() {}\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"graph", "diff", "--before", before, "--after", after}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"graph-diff files=1->1 declarations=+1/-1/~0/>0 calls=+1/-1/~0", "+ D go func added", "- D go func removed", "+ C", "- C"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("diff missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "grepple-navigation-diff-v5") || strings.Contains(output, "sources=before") {
		t.Fatalf("diff repeats machine metadata on a complete comparison:\n%s", output)
	}
	jsonOutput := captureStdout(t, func() {
		if err := Run([]string{"graph", "diff", "--before", before, "--after", after, "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var decoded graphDiffOutput
	if err := json.Unmarshal([]byte(jsonOutput), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.MovedDeclarations) != 0 || decoded.Schema != search.NavigationDiffSchema || decoded.BeforeSources.Parsed != 1 || decoded.AfterSources.Parsed != 1 {
		t.Fatalf("unexpected diff=%#v", decoded)
	}
}

func TestGraphResolvePreviewsDeterministicAtAlternatives(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "workers.go", "package sample\ntype Alpha struct{}\ntype Beta struct{}\nfunc (Alpha) Run() {}\nfunc (Beta) Run() {}\n")
	jsonText := captureStdout(t, func() {
		if err := Run([]string{"graph", "resolve", "--symbol", "Run", "--json", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var output graphResolveOutput
	if err := json.Unmarshal([]byte(jsonText), &output); err != nil {
		t.Fatal(err)
	}
	if output.Schema != navigationResolveSchema || len(output.Matches) != 2 || output.Matches[0].Name != "Alpha.Run" || output.Matches[1].Name != "Beta.Run" {
		t.Fatalf("unexpected resolve output: %#v", output)
	}
	for _, match := range output.Matches {
		if match.ID == "" || match.At == "" || !strings.Contains(match.CallersCommand, "graph callers --at") || !strings.Contains(match.CalleesCommand, "graph callees --at") {
			t.Fatalf("resolve match is not actionable: %#v", match)
		}
	}

	compact := captureStdout(t, func() {
		if err := Run([]string{"graph", "resolve", "--symbol", "Alpha.Run", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.HasPrefix(compact, "D ") || !strings.Contains(compact, "at: workers.go:4") || !strings.Contains(compact, "grepple graph callees --at workers.go:4") {
		t.Fatalf("compact resolve output missing exact alternative:\n%s", compact)
	}
}

func TestGraphResolveDefaultsToHumanOutputAndRequiresSelector(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", "package sample\nfunc Run() {}\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"graph", "resolve", "--symbol", "Run"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.HasPrefix(output, "D ") || !strings.Contains(output, "func Run") {
		t.Fatalf("default resolve output = %q", output)
	}
	if err := Run([]string{"graph", "resolve"}); err == nil {
		t.Fatal("expected missing --symbol error")
	}
}

func TestGraphRecursiveHelpStatesOutputContract(t *testing.T) {
	tests := [][]string{
		{"help", "graph", "callers"},
		{"help", "graph", "callees"},
		{"help", "graph", "resolve"},
	}
	for _, args := range tests {
		output := captureStdout(t, func() {
			if err := Run(args); err != nil {
				t.Fatalf("Run(%q): %v", args, err)
			}
		})
		if strings.Contains(output, "--compact") || !strings.Contains(output, "--json") {
			t.Fatalf("Run(%q) should show --json as the optional output mode:\n%s", args, output)
		}
	}
}

func TestGraphRemovedTraversalCommandsFailClearly(t *testing.T) {
	for _, name := range []string{"impact", "dependencies", "dependents"} {
		if err := Run([]string{"graph", name, "--symbol", "Run"}); err == nil || !strings.Contains(err.Error(), "has been removed") {
			t.Fatalf("graph %s returned %v", name, err)
		}
		if err := Run([]string{"graph", name, "--help"}); err == nil || !strings.Contains(err.Error(), "has been removed") {
			t.Fatalf("graph %s help returned %v", name, err)
		}
	}
}

func TestGraphReportsDiscoveredParsedSkippedFailedAndRecoveredSources(t *testing.T) {
	root := t.TempDir()
	writeGraphSource(t, root, "valid.go", "package sample\nfunc Valid() {}\n")
	writeGraphSource(t, root, "recovered.go", "package sample\nfunc Recovered( {\n")
	writeGraphSource(t, root, "binary.go", "package sample\x00")
	if err := os.WriteFile(filepath.Join(root, "invalid.go"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	writeGraphSource(t, root, "notes.md", "# not navigation source\n")

	outputText := captureStdout(t, func() {
		if err := Run([]string{"graph", "--json", root}); err != nil {
			t.Fatal(err)
		}
	})
	var output navigationGraphOutput
	if err := json.Unmarshal([]byte(outputText), &output); err != nil {
		t.Fatal(err)
	}
	want := navigationSourceSummary{Discovered: 5, Selected: 4, Parsed: 2, Skipped: 2, Failed: 1, Recovered: 1}
	if output.Sources != want {
		t.Fatalf("sources=%#v, want %#v", output.Sources, want)
	}

	compact := captureStdout(t, func() {
		if err := Run([]string{"graph", root}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(compact, "! incomplete sources=discovered:5,selected:4,parsed:2,skipped:2,failed:1,recovered:1") {
		t.Fatalf("compact source completeness missing:\n%s", compact)
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

func assertGraphResolutionStats(t *testing.T, stats search.NavigationResolutionStats) {
	t.Helper()
	if stats.Calls != 1 || stats.Resolved != 1 || stats.Ambiguous != 0 || stats.ResolvedLocal != 1 || stats.ResolutionRate != 1 || len(stats.Outcomes) != 1 || stats.Outcomes[0] != (search.NavigationResolutionCount{Name: "resolved-local", Count: 1}) || len(stats.Languages) != 1 || stats.Languages[0].Language != "go" || stats.Languages[0].ResolvedLocal != 1 {
		t.Fatalf("graph resolution stats=%#v", stats)
	}
}
