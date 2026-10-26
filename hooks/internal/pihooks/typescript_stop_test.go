package pihooks

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestHandleLintHookTypeScriptOnlySkipsMermaidSchemas(t *testing.T) {
	tests := []struct{ name, diagramName, source, diagram string }{
		{"class", "model.class.mmd", "class Model { value: number = 1; }\n", "classDiagram\n class Model {\n  +value: string\n }\n <<typescript>> Model\n"},
		{"flow", "calls.flow.mmd", "function start(): void {}\n", "flowchart TD\n start[\"start\"]\n finish[\"finish\"]\n start --> finish\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeHookTestFile(t, filepath.Join(root, "code.ts"), test.source)
			writeHookTestFile(t, filepath.Join(root, test.diagramName), test.diagram)
			if output := HandleLintHook(stopHookInput(t, root), filepath.Join("..", "..")); len(output) != 0 {
				t.Fatalf("unexpected Stop feedback: %s", output)
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

func TestHandleLintHookTypeScriptOnlyMalformedSchemaIsIgnored(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.ts"), "class Model {}\n")
	writeHookTestFile(t, filepath.Join(root, "broken.class.mmd"), "classDiagram\n class Broken {\n")
	if output := HandleLintHook(stopHookInput(t, root), filepath.Join("..", "..")); len(output) != 0 {
		t.Fatalf("unexpected Stop feedback: %s", output)
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
