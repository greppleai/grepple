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
	if err := runExtract([]string{"structure", filepath.Dir(source), "--output", output}); err != nil {
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

func TestExtractStructureWritesCanonicalBundle(t *testing.T) {
	root, source := writeExtractFixture(t)
	output := filepath.Join(root, "service.package")
	if err := runExtract([]string{"structure", filepath.Dir(source), "--bundle", "--output", output}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest.json", "overview.mmd", "structure.mmd"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil || !strings.Contains(string(manifest), `"line": 3`) {
		t.Fatalf("manifest location: %v\n%s", err, manifest)
	}
	structure, err := os.ReadFile(filepath.Join(output, "structure.mmd"))
	if err != nil || !strings.Contains(string(structure), "defined: service/service.go:3") {
		t.Fatalf("structure location: %v\n%s", err, structure)
	}
	if err := runExtract([]string{"check", "package", output, filepath.Dir(source)}); err != nil {
		t.Fatal(err)
	}
}

func TestExtractRejectsInvalidModeCombinations(t *testing.T) {
	if err := runExtract([]string{"flow", "--bundle", "--entry", "Run"}); err == nil || !strings.Contains(err.Error(), "only to extract structure") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := runExtract([]string{"flow", "--entry", "Run", "--at", "x.go:1"}); err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
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
