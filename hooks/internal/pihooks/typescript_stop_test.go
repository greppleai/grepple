package pihooks

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleLintHookValidatesTypeScriptOnlySchemas(t *testing.T) {
	tests := []struct{ name, diagramName, source, diagram, expected string }{
		{"class", "model.class.mmd", "class Model { value: number = 1; }\n", "classDiagram\n class Model {\n  +value: string\n }\n <<typescript>> Model\n", "Expected public value: string"},
		{"flow", "calls.flow.mmd", "function start(): void {}\n", "flowchart TD\n start[\"start\"]\n finish[\"finish\"]\n start --> finish\n", "Missing code symbol 'finish'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeHookTestFile(t, filepath.Join(root, "code.ts"), test.source)
			writeHookTestFile(t, filepath.Join(root, test.diagramName), test.diagram)
			output := HandleLintHook(stopHookInput(t, root), filepath.Join("..", ".."))
			if !strings.Contains(string(output), "mermaid-code") || !strings.Contains(string(output), test.expected) {
				t.Fatalf("missing TS-only feedback: %s", output)
			}
		})
	}
}

func TestHandleLintHookTypeScriptOnlyNoSchemaIsClean(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "broken.ts"), "class Broken {")
	if output := HandleLintHook(stopHookInput(t, root), filepath.Join("..", "..")); len(output) != 0 {
		t.Fatalf("no-schema output = %s", output)
	}
}

func TestHandleLintHookTypeScriptOnlyMalformedSchemaIsBounded(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.ts"), "class Model {}\n")
	writeHookTestFile(t, filepath.Join(root, "broken.class.mmd"), "classDiagram\n class Broken {\n")
	output := HandleLintHook(stopHookInput(t, root), filepath.Join("..", ".."))
	if len(output) == 0 || len(output) > maxFeedbackBytes+1000 {
		t.Fatalf("malformed feedback size = %d", len(output))
	}
	if !strings.Contains(string(output), "Mermaid code schema check failed") || !strings.Contains(string(output), "missing '}'") {
		t.Fatalf("unexpected malformed feedback: %s", output)
	}
}

func stopHookInput(t *testing.T, cwd string) []byte {
	t.Helper()
	input, err := json.Marshal(map[string]string{"hook_event_name": "Stop", "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	return input
}
