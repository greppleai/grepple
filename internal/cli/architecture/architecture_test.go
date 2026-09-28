package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

func TestArchitectureDirectoryAcrossLanguages(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, ".grepple/grepple.json", `{"ignore":{"paths":["sandbox/**"]}}`)
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "parser/document.go", "package parser\n// Document is public.\ntype Document struct{}\n// Build constructs a document.\nfunc Build() Document { return Document{} }\n")
	writeArchitectureFixture(t, root, "parser/document_test.go", "package parser\ntype TestDocument struct{}\n")
	writeArchitectureFixture(t, root, "web/component.ts", "export class Component { render() {} }\n")
	writeArchitectureFixture(t, root, "sandbox/ignored.py", "class Ignored:\n    pass\n")
	chdirForConfigTest(t, root)

	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--json", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var architecture directoryArchitecture
	if err := json.Unmarshal([]byte(output), &architecture); err != nil {
		t.Fatal(err)
	}
	if architecture.Schema != directoryArchitectureSchema || architecture.Files != 3 || len(architecture.Directories) != 3 {
		t.Fatalf("architecture = %+v", architecture)
	}
	assertArchitectureSourceFiles(t, architecture.SourceFiles)
	if formatArchitectureCounts(architecture.Directories[0].Classifications) != "production:2,test:1" || architecture.Directories[0].PublicCallables != 1 {
		t.Fatalf("root classifications=%+v", architecture.Directories[0].Classifications)
	}
	for _, directory := range architecture.Directories {
		if strings.HasPrefix(directory.Path, "sandbox") {
			t.Fatalf("ignored directory included: %+v", directory)
		}
	}

}

func TestArchitectureDaemonUnavailableFallsBackToDirect(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "main.go", "package main\nfunc main() {}\n")
	chdirForConfigTest(t, root)
	t.Setenv("GREPPLE_CACHE_DIR", filepath.Join(t.TempDir(), "absent-cache"))
	direct, err := Build([]string{"."}, 0, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := Build([]string{"."}, 0, Dependencies{Daemon: true})
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(direct)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(fallback)
	if err != nil || string(got) != string(want) {
		t.Fatalf("unavailable daemon changed report: %v", err)
	}
}

func assertArchitectureSourceFiles(t *testing.T, files []architectureSourceFile) {
	t.Helper()
	if len(files) != 3 {
		t.Fatalf("source files = %+v", files)
	}
	if files[0].Path != "parser/document.go" || files[0].Classification != "production" || files[1].Classification != "test" || files[2].Language != "typescript" {
		t.Fatalf("source files = %+v", files)
	}
}

func TestArchitectureDirectoryReportsAdapterEvidencedEntrypoints(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "cmd/server/main.go", "package main\nfunc main() {}\n")
	chdirForConfigTest(t, root)
	compact := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"entrypoints=1", "E process main cmd/server/main.go:2"} {
		if !strings.Contains(compact, expected) {
			t.Fatalf("compact architecture missing %q:\n%s", expected, compact)
		}
	}
	architecture, err := buildDirectoryArchitecture([]string{"."}, 0)
	if err != nil {
		t.Fatal(err)
	}
	matches := analysis.ResolveArchitectureSymbols(architecture.Symbols, "main")
	if len(matches) != 1 || matches[0].Entrypoint != "process" {
		t.Fatalf("matches=%+v", matches)
	}
}

func TestArchitectureDirectoryCapturesResolvedCrossDirectoryCalls(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "search/request.go", "package search\ntype Request struct{}\nfunc ResolveRequest() {}\n")
	writeArchitectureFixture(t, root, "rulespec/rule.go", "package rulespec\nimport \"example.com/project/search\"\nfunc Validate(value search.Request) { search.ResolveRequest() }\n")
	writeArchitectureFixture(t, root, "rulespec/rule_test.go", "package rulespec\nimport \"example.com/project/search\"\nfunc TestValidate(value search.Request) { search.ResolveRequest() }\n")
	chdirForConfigTest(t, root)
	assertArchitectureRelationCoverage(t)

	architecture, err := buildDirectoryArchitecture([]string{"."}, 0)
	if err != nil {
		t.Fatal(err)
	}
	evidence := analysis.ArchitectureRelationEvidenceFor(architecture.Relations, "rulespec", "search")
	if got := analysis.ArchitectureEvidenceRelation(evidence); got != "import,resolved-call,type-reference" || len(evidence) != 6 {
		t.Fatalf("relation=%s evidence=%d", got, len(evidence))
	}
	for _, kind := range []string{"import", "resolved-call", "type-reference"} {
		if !slices.ContainsFunc(evidence, func(item analysis.ArchitectureRelationEvidence) bool {
			return item.Kind == kind && item.Classification == "production" && item.Path == "rulespec/rule.go"
		}) {
			t.Fatalf("missing production %s evidence: %+v", kind, evidence)
		}
	}
}

func TestArchitectureDirectoryCapturesImportOnlyRelation(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "target/data.go", "package target\nconst Value = 1\n")
	writeArchitectureFixture(t, root, "side/effect.go", "package side\nimport _ \"example.com/project/target\"\n")
	chdirForConfigTest(t, root)
	report, err := buildDirectoryArchitecture([]string{"."}, 0)
	if err != nil {
		t.Fatal(err)
	}
	evidence := analysis.ArchitectureRelationEvidenceFor(report.Relations, "side", "target")
	if got := analysis.ArchitectureEvidenceRelation(evidence); got != "import" || len(evidence) != 1 || evidence[0].Path != "side/effect.go" {
		t.Fatalf("relation=%s evidence=%+v", got, evidence)
	}
}

func TestArchitectureHelpStopsBeforeAnalysis(t *testing.T) {
	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--help"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(output, "--compact") || !strings.Contains(output, "--json") {
		t.Fatalf("directory help output:\n%s", output)
	}
}

func assertArchitectureRelationCoverage(t *testing.T) {
	t.Helper()
	architecture, err := buildDirectoryArchitecture([]string{"."}, 0)
	if err != nil {
		t.Fatal(err)
	}
	coverage := architecture.RelationCoverage
	if coverage.ImportFacts != 2 || coverage.ResolvedImports != 2 || coverage.TypeReferences != 2 || coverage.ResolvedTypeReferences != 2 {
		t.Fatalf("coverage=%+v", coverage)
	}
	matched := 0
	for _, relation := range architecture.Relations {
		if relation.From != "rulespec" || relation.To != "search" {
			continue
		}
		matched++
		if formatArchitectureCounts(relation.Classifications) != "production:1,test:1" {
			t.Fatalf("relation classifications=%+v relation=%+v", relation.Classifications, relation)
		}
	}
	if matched != 3 {
		t.Fatalf("rulespec -> search relation kinds=%d, want 3", matched)
	}
}

func writeArchitectureFixture(t testing.TB, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(relative, ".grepple/") && filepath.Base(path) != directorymeta.FileName {
		writeArchitectureFixtureMetadata(t, root, path)
	}
}

func writeArchitectureFixtureMetadata(t testing.TB, root, path string) {
	t.Helper()
	directory, name := filepath.Dir(path), filepath.Base(path)
	metadata, err := directorymeta.Read(root, directory)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if os.IsNotExist(err) {
		metadata = directorymeta.Metadata{Description: "Architecture fixtures.", Responsibilities: []string{"Test architecture analysis."}}
	}
	digest, err := filedigest.SHA256Hex(path)
	if err != nil {
		t.Fatal(err)
	}
	kind := "production"
	if strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
		kind = "test"
	}
	entry := directorymeta.File{Path: name, Description: "Architecture fixture.", Kind: kind, Checksum: digest}
	updated := false
	for index := range metadata.Files {
		if metadata.Files[index].Path == name {
			metadata.Files[index] = entry
			updated = true
		}
	}
	if !updated {
		metadata.Files = append(metadata.Files, entry)
	}
	if err := directorymeta.Write(root, directory, metadata); err != nil {
		t.Fatal(err)
	}
}
