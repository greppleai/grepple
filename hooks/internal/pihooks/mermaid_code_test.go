package pihooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeMermaidSchemasClassAndFlowMismatches(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.go"), "package sample\ntype Worker struct { Count int }\nfunc start() {}\n")
	writeHookTestFile(t, filepath.Join(root, "worker.structure.mmd"), "classDiagram\n class Worker {\n  +Count: string\n }\n <<struct>> Worker\n")
	writeHookTestFile(t, filepath.Join(root, "calls.flow.mmd"), "flowchart TD\n start[\"start\"]\n finish[\"finish\"]\n start --> finish\n")
	diagnostics, err := AnalyzeMermaidSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("got diagnostics: %+v", diagnostics)
	}
	messages := diagnostics[0].Failure + "\n" + diagnostics[1].Failure
	if !strings.Contains(messages, "Expected public Count: string") || !strings.Contains(messages, "Missing code symbol 'finish'") {
		t.Fatalf("missing class/flow mismatches: %s", messages)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.RuleName != "mermaid-code" || diagnostic.Position.Start.Line == 0 {
			t.Fatalf("invalid diagnostic: %+v", diagnostic)
		}
	}
}

func TestAnalyzeMermaidSchemasNoSchemaDoesNotDiscoverSources(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "broken.go"), "package broken\ntype Broken struct {")
	writeHookTestFile(t, filepath.Join(root, "generated", "ignored.class.mmd"), "not Mermaid")
	diagnostics, err := AnalyzeMermaidSchemas(root)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("no-schema result: %+v, %v", diagnostics, err)
	}
}

func TestAnalyzeMermaidSchemasSurfacesSyntaxErrors(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.ts"), "class Good {}\n")
	path := filepath.Join(root, "broken.class.mmd")
	writeHookTestFile(t, path, "classDiagram\n class Broken {\n")
	_, err := AnalyzeMermaidSchemas(root)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "missing '}'") {
		t.Fatalf("unexpected syntax error: %v", err)
	}
}

func TestAnalyzeMermaidSchemasDeterministicOrdering(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.go"), "package sample\n")
	writeHookTestFile(t, filepath.Join(root, "z.class.mmd"), "classDiagram\n class Zed\n <<struct>> Zed\n")
	writeHookTestFile(t, filepath.Join(root, "a.flow.mmd"), "flowchart TD\n absent[\"absent\"]\n")
	first, err := AnalyzeMermaidSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeMermaidSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("unexpected diagnostics: %+v", first)
	}
	for index := range first {
		if first[index].Position.Start.Filename != second[index].Position.Start.Filename || first[index].Failure != second[index].Failure {
			t.Fatal("ordering changed")
		}
	}
	if filepath.Base(first[0].Position.Start.Filename) != "a.flow.mmd" {
		t.Fatalf("not path sorted: %+v", first)
	}
}

func TestLintEvaluationAppendsMermaidDiagnostics(t *testing.T) {
	root := makeRepository(t, map[string]string{"code.go": "package sample\n"})
	writeHookTestFile(t, filepath.Join(root, "code.go"), "package sample\n")
	writeHookTestFile(t, filepath.Join(root, "missing.class.mmd"), "classDiagram\n class Missing\n <<struct>> Missing\n")
	output := evaluateLintResult(commandResult{stdout: "[]"}, root, filepath.Join("..", ".."), nil)
	if !strings.Contains(string(output), "mermaid-code") || !strings.Contains(string(output), "Missing struct") {
		t.Fatalf("schema diagnostic not in Stop feedback: %s", output)
	}
}

func TestAnalyzeMermaidSchemasSkipsGeneratedTreeSitterParser(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.go"), "package sample\ntype Worker struct{}\n")
	writeHookTestFile(t, filepath.Join(root, "worker.structure.mmd"), "classDiagram\n class Worker\n <<struct>> Worker\n")
	writeHookTestFile(t, filepath.Join(root, "gritgrammar", "parser.c"), strings.Join([]string{
		"#define LANGUAGE_VERSION 14", "#define STATE_COUNT 2", "static const int ts_lex_modes[1] = {", "BROKEN }", "const TSLanguage *tree_sitter_test(void);",
	}, "\n"))
	writeHookTestFile(t, filepath.Join(root, "gritgrammar", "tree_sitter", "alloc.h"), "typedef broken }\n#endif\n")
	diagnostics, err := AnalyzeMermaidSchemas(root)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("generated parser affected schema analysis: diagnostics=%+v err=%v", diagnostics, err)
	}
}

func TestAnalyzeMermaidSchemasSurfacesSourceErrors(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "broken.go"), "package broken\ntype Broken struct {")
	writeHookTestFile(t, filepath.Join(root, "broken.class.mmd"), "classDiagram\n class Broken\n <<struct>> Broken\n")
	_, err := AnalyzeMermaidSchemas(root)
	if err == nil || !strings.Contains(err.Error(), "broken.go") || !strings.Contains(err.Error(), "malformed syntax") {
		t.Fatalf("unexpected source error: %v", err)
	}
}

func writeHookTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
