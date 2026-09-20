package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestDirectoryArchitectureColdWarmComparisonIsByteIdentical(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "cmd/server/main.go", "package main\nfunc main() { run() }\nfunc run() {}\n")
	chdirForConfigTest(t, root)
	build := func() string {
		return captureStdout(t, func() {
			if err := runArchitecture([]string{"directory", "--json", "."}); err != nil {
				t.Fatal(err)
			}
		})
	}
	beforeBytes, afterBytes := []byte(build()), []byte(build())
	var before, after directoryArchitecture
	if err := json.Unmarshal(beforeBytes, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(afterBytes, &after); err != nil {
		t.Fatal(err)
	}
	comparison := compareDirectoryArchitectures("cold.json", "warm.json", beforeBytes, afterBytes, before, after)
	if !comparison.SemanticEqual || !comparison.ByteEqual || comparison.Difference != nil {
		t.Fatalf("cold/warm comparison=%+v", comparison)
	}
}
func TestCompareDirectoryArchitecturesNormalizesBeforeByteCheck(t *testing.T) {
	before := architectureComparisonFixture()
	before.Root = `.\`
	before.SourceFiles[0].Path = `service\service.go`
	before.Symbols[0].Path = `service\service.go`
	before.Symbols[0].Directory = `service\.`
	before.Directories[1].Languages = []architectureCount{{Name: "typescript", Count: 1}, {Name: "go", Count: 1}}
	after := architectureComparisonFixture()
	after.Directories[1].Languages = []architectureCount{{Name: "go", Count: 1}, {Name: "typescript", Count: 1}}
	beforeBytes := marshalArchitectureFixture(t, before, false)
	afterBytes := marshalArchitectureFixture(t, after, true)

	comparison := compareDirectoryArchitectures("before.json", "after.json", beforeBytes, afterBytes, before, after)
	if !comparison.SemanticEqual || comparison.ByteEqual {
		t.Fatalf("comparison=%+v", comparison)
	}
	if comparison.Difference == nil || comparison.Difference.Kind != "encoding" || comparison.Difference.ByteOffset == nil {
		t.Fatalf("difference=%+v", comparison.Difference)
	}
}

func TestCompareDirectoryArchitecturesReportsFirstSourceLinkedFact(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*directoryArchitecture)
		kind     string
		identity string
		path     string
	}{
		{name: "file", mutate: func(value *directoryArchitecture) { value.SourceFiles[0].Classification = "test" }, kind: "file", identity: "service/service.go", path: "service/service.go"},
		{name: "declaration", mutate: func(value *directoryArchitecture) { value.Symbols[0].End++ }, kind: "declaration", identity: "service/service.go|go|func||Run", path: "service/service.go"},
		{name: "relation", mutate: func(value *directoryArchitecture) { value.Relations[0].Count++ }, kind: "relation", identity: "app|service|resolved-call", path: "app/main.go"},
		{name: "directory", mutate: func(value *directoryArchitecture) { value.Directories[1].PublicCallables++ }, kind: "directory", identity: "service", path: "service"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := architectureComparisonFixture()
			after := cloneArchitectureFixture(t, before)
			test.mutate(&after)
			comparison := compareDirectoryArchitectures("before.json", "after.json", marshalArchitectureFixture(t, before, false), marshalArchitectureFixture(t, after, false), before, after)
			if comparison.SemanticEqual || comparison.Difference == nil {
				t.Fatalf("comparison=%+v", comparison)
			}
			difference := comparison.Difference
			if difference.Kind != test.kind || difference.Change != "changed" || difference.Identity != test.identity || difference.Path != test.path {
				t.Fatalf("difference=%+v", difference)
			}
			if len(difference.Before) == 0 || len(difference.After) == 0 {
				t.Fatalf("difference lacks before/after values: %+v", difference)
			}
		})
	}
}

func TestArchitectureCompareCommandReportsSemanticDifference(t *testing.T) {
	before := architectureComparisonFixture()
	after := cloneArchitectureFixture(t, before)
	after.Symbols[0].Visibility = "private"
	beforePath := filepath.Join(t.TempDir(), "before.json")
	afterPath := filepath.Join(t.TempDir(), "after.json")
	if err := os.WriteFile(beforePath, marshalArchitectureFixture(t, before, false), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, marshalArchitectureFixture(t, after, false), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		err := Run([]string{"architecture", "compare", "--compact", beforePath, afterPath, "--no-spill"})
		if code, ok := ExitCode(err); !ok || code != 1 {
			t.Fatalf("compare error=%v", err)
		}
	})
	if !strings.Contains(output, "semantic-equal=false byte-equal=false") || !strings.Contains(output, "! declaration changed") || !strings.Contains(output, "at=service/service.go:3") {
		t.Fatalf("compare output:\n%s", output)
	}
}

func TestReadDirectoryArchitectureRejectsUnknownSchemaAndTrailingJSON(t *testing.T) {
	value := architectureComparisonFixture()
	value.Schema = "grepple-directory-architecture-v3"
	path := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(path, marshalArchitectureFixture(t, value, false), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readDirectoryArchitecture(path); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("schema error=%v", err)
	}
	value.Schema = directoryArchitectureSchema
	value.SourceFiles = nil
	if err := os.WriteFile(path, marshalArchitectureFixture(t, value, false), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readDirectoryArchitecture(path); err == nil || !strings.Contains(err.Error(), "sourceFiles") {
		t.Fatalf("source inventory error=%v", err)
	}
	value = architectureComparisonFixture()
	content := append(marshalArchitectureFixture(t, value, false), []byte("{}")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readDirectoryArchitecture(path); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("trailing JSON error=%v", err)
	}
}

func architectureComparisonFixture() directoryArchitecture {
	return directoryArchitecture{
		Schema: directoryArchitectureSchema,
		Root:   ".",
		Files:  1,
		Sources: architectureSourceSummary{
			Discovered: 1, Selected: 1, Parsed: 1,
		},
		SourceFiles: []architectureSourceFile{{Path: "service/service.go", Language: "go", Classification: "production"}},
		Directories: []architectureDirectory{
			{Path: ".", Files: 1, Classifications: []architectureCount{{Name: "production", Count: 1}}, Languages: []architectureCount{{Name: "go", Count: 1}}, Declarations: []architectureCount{{Name: "func", Count: 1}}, PublicCallables: 1},
			{Path: "service", Files: 1, Classifications: []architectureCount{{Name: "production", Count: 1}}, Languages: []architectureCount{{Name: "go", Count: 1}, {Name: "typescript", Count: 1}}, Declarations: []architectureCount{{Name: "func", Count: 1}}, PublicCallables: 1},
		},
		Symbols:         []architectureSymbol{{Name: "Run", Kind: "func", Language: "go", Classification: "production", Path: "service/service.go", Directory: "service", Visibility: "public", Start: 3, End: 5}},
		Relations:       []architectureRelation{{From: "app", To: "service", Kind: "resolved-call", Count: 1, Classifications: []architectureCount{{Name: "production", Count: 1}}, Evidence: []architectureRelationEvidence{{Path: "app/main.go", Line: 7, Caller: "main", Target: "Run", Kind: "resolved-call", Classification: "production", Confidence: "import-resolved"}}}},
		RepositoryRoots: []string{"example.com/project"},
	}
}

func cloneArchitectureFixture(t *testing.T, value directoryArchitecture) directoryArchitecture {
	t.Helper()
	var result directoryArchitecture
	if err := json.Unmarshal(marshalArchitectureFixture(t, value, false), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func marshalArchitectureFixture(t *testing.T, value directoryArchitecture, indent bool) []byte {
	t.Helper()
	var content []byte
	var err error
	if indent {
		content, err = json.MarshalIndent(value, "", "  ")
	} else {
		content, err = json.Marshal(value)
	}
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestReadDirectoryArchitectureAcceptsRemoteEnvelope(t *testing.T) {
	architecture := architectureComparisonFixture()
	result := marshalArchitectureFixture(t, architecture, false)
	envelope, err := json.Marshal(api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: api.AnalysisArchitecture, Repository: "owner/repo", Found: true, Complete: true, Result: result})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "remote.json")
	if err := os.WriteFile(path, envelope, 0o600); err != nil {
		t.Fatal(err)
	}
	decoded, raw, err := readDirectoryArchitecture(path)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != directoryArchitectureSchema || string(raw) != string(result) {
		t.Fatalf("decoded=%+v raw=%s", decoded, raw)
	}
}
