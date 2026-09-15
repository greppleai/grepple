package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureDirectoryAndResolveAcrossLanguages(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "grepple.json", `{"ignore":{"paths":["sandbox/**"]}}`)
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

	resolved := captureStdout(t, func() {
		if err := runArchitecture([]string{"resolve", "--symbol", "Document", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(resolved, "matches=1") || !strings.Contains(resolved, "parser/document.go:3") || !strings.Contains(resolved, "class=production") {
		t.Fatalf("resolve output:\n%s", resolved)
	}

	assertProductionOnlyArchitectureResolve(t)
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

func TestArchitectureDirectoryReportsAdapterEvidencedEntrypointsAndRoutes(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "cmd/server/main.go", "package main\nimport web \"net/http\"\nfunc main() { web.HandleFunc(\"POST /items\", createItem) }\nfunc createItem(web.ResponseWriter, *web.Request) {}\n")
	chdirForConfigTest(t, root)
	compact := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"entrypoints=1 routes=1", "E process main cmd/server/main.go:3", "P POST /items -> createItem framework=net/http at=cmd/server/main.go:3"} {
		if !strings.Contains(compact, expected) {
			t.Fatalf("compact architecture missing %q:\n%s", expected, compact)
		}
	}
	architecture, err := buildDirectoryArchitecture([]string{"."}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(architecture.Routes) != 1 || architecture.Routes[0].Classification != "production" || architecture.Routes[0].Directory != "cmd/server" {
		t.Fatalf("routes=%+v", architecture.Routes)
	}
	matches := resolveArchitectureSymbols(architecture.Symbols, "main")
	if len(matches) != 1 || matches[0].Entrypoint != "process" {
		t.Fatalf("matches=%+v", matches)
	}
}

func assertProductionOnlyArchitectureResolve(t *testing.T) {
	t.Helper()
	production := captureStdout(t, func() {
		err := Run([]string{"architecture", "resolve", "--symbol", "TestDocument", "--compact", "--production-only", ".", "--no-spill"})
		if code, ok := ExitCode(err); !ok || code != 1 {
			t.Fatalf("production-only resolve error=%v", err)
		}
	})
	if !strings.Contains(production, "matches=0") {
		t.Fatalf("production-only resolve output:\n%s", production)
	}
}

func TestArchitectureWhyUsesResolvedCrossDirectoryCalls(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "search/request.go", "package search\ntype Request struct{}\nfunc ResolveRequest() {}\n")
	writeArchitectureFixture(t, root, "rulespec/rule.go", "package rulespec\nimport \"example.com/project/search\"\nfunc Validate(value search.Request) { search.ResolveRequest() }\n")
	writeArchitectureFixture(t, root, "rulespec/rule_test.go", "package rulespec\nimport \"example.com/project/search\"\nfunc TestValidate(value search.Request) { search.ResolveRequest() }\n")
	chdirForConfigTest(t, root)
	assertArchitectureRelationCoverage(t)

	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"why", "rulespec", "search", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		"relation=import,resolved-call,type-reference evidence=6",
		"rulespec/rule.go:2 search -> example.com/project/search kind=import class=production confidence=local-import-resolved",
		"rulespec/rule.go:3 Validate -> ResolveRequest kind=resolved-call class=production confidence=import-resolved",
		"rulespec/rule.go:3 Validate -> Request kind=type-reference class=production confidence=local-import-resolved",
		"rulespec/rule_test.go:3 TestValidate -> ResolveRequest kind=resolved-call class=test confidence=import-resolved",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("why output missing %q:\n%s", expected, output)
		}
	}
}

func TestArchitectureWhyReportsImportOnlyRelation(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "target/data.go", "package target\nconst Value = 1\n")
	writeArchitectureFixture(t, root, "side/effect.go", "package side\nimport _ \"example.com/project/target\"\n")
	chdirForConfigTest(t, root)
	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"why", "side", "target", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "relation=import evidence=1") || !strings.Contains(output, "side/effect.go:2 _ -> example.com/project/target kind=import") {
		t.Fatalf("import-only why output:\n%s", output)
	}
}

func TestArchitectureHelpStopsBeforeAnalysis(t *testing.T) {
	for _, command := range []string{"directory", "compare"} {
		output := captureStdout(t, func() {
			if err := runArchitecture([]string{command, "--help"}); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(output, "--compact") || strings.Contains(output, "requires exactly one") {
			t.Fatalf("%s help output:\n%s", command, output)
		}
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
}
