package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractStructureGeneratesValidatedGoDiagram(t *testing.T) {
	root, source := writeExtractFixture(t)
	output := filepath.Join(root, "structure.mmd")
	if err := runExtract([]string{"structure", "--entry", "Service", "--source", filepath.Dir(source), "--output", output}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "classDiagram") || !strings.Contains(text, "class Service") || !strings.Contains(text, "defined: service/service.go:3") {
		t.Fatalf("unexpected diagram:\n%s", text)
	}
	if err := runExtract([]string{"check", "structure", output, filepath.Dir(source)}); err != nil {
		t.Fatal(err)
	}
}

func TestExtractFlowSupportsEntryAndAt(t *testing.T) {
	root, source := writeExtractFixture(t)
	for name, selector := range map[string][]string{
		"entry": {"--entry", "Service.Run"},
		"at":    {"--at", source + ":6"},
	} {
		t.Run(name, func(t *testing.T) {
			output := filepath.Join(root, name+".mmd")
			arguments := append([]string{"flow", filepath.Dir(source)}, selector...)
			arguments = append(arguments, "--output", output)
			if err := runExtract(arguments); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), "Service.Run() · service/service.go:6") || !strings.Contains(string(content), "Service_Run --> helper") {
				t.Fatalf("unexpected flow:\n%s", content)
			}
			if err := runExtract([]string{"check", "flow", output, filepath.Dir(source)}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExtractFocusedCommandsUseLanguageAdapters(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "main.ts")
	content := "export interface Store { load(): Item }\nexport class Item { value: string }\nexport function start(): void { finish() }\nfunction finish(): void {}\n"
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	structure := filepath.Join(root, "store.structure.mmd")
	if err := runExtract([]string{"structure", "--at", source + ":1", "--source", root, "--output", structure}); err != nil {
		t.Fatal(err)
	}
	structureContent, err := os.ReadFile(structure)
	if err != nil || !strings.Contains(string(structureContent), "defined: main.ts:1") || !strings.Contains(string(structureContent), "<<typescript>> Store") {
		t.Fatalf("TypeScript structure: %v\n%s", err, structureContent)
	}
	flow := filepath.Join(root, "start.flow.mmd")
	if err := runExtract([]string{"flow", "--at", source + ":3", "--source", root, "--depth", "1", "--output", flow}); err != nil {
		t.Fatal(err)
	}
	flowContent, err := os.ReadFile(flow)
	if err != nil || !strings.Contains(string(flowContent), "start() · main.ts:3") || !strings.Contains(string(flowContent), "start --> finish") {
		t.Fatalf("TypeScript flow: %v\n%s", err, flowContent)
	}
}

func TestExtractFocusedCommandsSupportPython(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "worker.py")
	content := "class Worker:\n    def run(self):\n        finish()\n\ndef finish():\n    pass\n"
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	structure := filepath.Join(root, "worker.structure.mmd")
	if err := runExtract([]string{"structure", "--at", source + ":1", "--source", root, "--output", structure}); err != nil {
		t.Fatal(err)
	}
	structureContent, err := os.ReadFile(structure)
	if err != nil || !strings.Contains(string(structureContent), "<<python>> Worker") {
		t.Fatalf("Python structure: %v\n%s", err, structureContent)
	}
	byName := filepath.Join(root, "worker-by-name.structure.mmd")
	if err := runExtract([]string{"structure", "--entry", "Worker", "--source", root, "--output", byName}); err != nil {
		t.Fatalf("Python structure by name: %v", err)
	}
	flow := filepath.Join(root, "run.flow.mmd")
	if err := runExtract([]string{"flow", "--at", source + ":2", "--source", root, "--depth", "1", "--output", flow}); err != nil {
		t.Fatal(err)
	}
	flowContent, err := os.ReadFile(flow)
	if err != nil || !strings.Contains(string(flowContent), "Worker_run --> finish") {
		t.Fatalf("Python flow: %v\n%s", err, flowContent)
	}
}

func TestExtractFocusedCommandsSupportJavaKotlinAndCSharp(t *testing.T) {
	tests := []struct {
		name, extension, content, language, edge string
	}{
		{"java", ".java", "class Worker {\n    void run() { finish(); }\n    void finish() {}\n}\n", "java", "Worker_run --> Worker_finish"},
		{"kotlin", ".kt", "class Worker {\n    fun run() { finish() }\n    fun finish() {}\n}\n", "kotlin", "Worker_run --> Worker_finish"},
		{"csharp", ".cs", "class Worker {\n    void Run() { Finish(); }\n    void Finish() {}\n}\n", "csharp", "Worker_Run --> Worker_Finish"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { assertFocusedJVMCLI(t, test.extension, test.content, test.language, test.edge) })
	}
}

func assertFocusedJVMCLI(t *testing.T, extension, content, language, edge string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "Worker"+extension)
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	structure := filepath.Join(root, "worker.structure.mmd")
	if err := runExtract([]string{"structure", "--at", source + ":1", "--source", root, "--output", structure}); err != nil {
		t.Fatal(err)
	}
	structureContent, err := os.ReadFile(structure)
	if err != nil || !strings.Contains(string(structureContent), "<<"+language+">> Worker") {
		t.Fatalf("%s structure: %v\n%s", language, err, structureContent)
	}
	flow := filepath.Join(root, "run.flow.mmd")
	if err := runExtract([]string{"flow", "--at", source + ":2", "--source", root, "--depth", "1", "--output", flow}); err != nil {
		t.Fatal(err)
	}
	flowContent, err := os.ReadFile(flow)
	if err != nil || !strings.Contains(string(flowContent), edge) {
		t.Fatalf("%s flow: %v\n%s", language, err, flowContent)
	}
}

func TestExtractRejectsInvalidModeCombinations(t *testing.T) {
	if err := runExtract([]string{"structure", filepath.Dir("x.go")}); err == nil || !strings.Contains(err.Error(), "requires --entry") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := runExtract([]string{"flow", "--entry", "Run", "--at", "x.go:1"}); err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractRecursiveHelp(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"extract", "--help"}, "grepple extract structure (--entry SYMBOL | --at PATH:LINE)"},
		{[]string{"help", "extract", "structure"}, "Usage: grepple extract structure"},
		{[]string{"help", "extract", "flow"}, "Usage: grepple extract flow"},
		{[]string{"help", "extract", "check"}, "grepple extract check <structure|flow>"},
		{[]string{"help", "extract", "check", "flow"}, "grepple extract check flow TARGET"},
	}
	for _, test := range tests {
		output := captureStdout(t, func() {
			if err := Run(test.args); err != nil {
				t.Fatalf("Run(%q): %v", test.args, err)
			}
		})
		if !strings.Contains(output, test.want) {
			t.Fatalf("Run(%q) missing %q:\n%s", test.args, test.want, output)
		}
	}
}

func writeExtractFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/extracttest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "service")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(directory, "service.go")
	content := "package service\n\ntype Service struct { Name string }\nfunc helper() {}\n\nfunc (Service) Run() { helper() }\n"
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, source
}
