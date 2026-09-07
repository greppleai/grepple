package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckContracts(t *testing.T) {
	fixtures := filepath.Join("..", "..", "internal", "mermaidcode", "testdata")
	if code := run([]string{"check", "class", filepath.Join(fixtures, "sample.mmd"), filepath.Join(fixtures, "sample.ts")}); code != 0 {
		t.Fatalf("valid class check returned %d", code)
	}
	if code := run([]string{"check", "structure", filepath.Join(fixtures, "sample.mmd"), filepath.Join(fixtures, "sample.ts")}); code != 0 {
		t.Fatalf("valid structure alias check returned %d", code)
	}
	if code := run([]string{"check", "flow", filepath.Join(fixtures, "pi-agent-tool-flow.mmd"), filepath.Join(fixtures, "sample.ts")}); code != 1 {
		t.Fatalf("mismatching flow check returned %d", code)
	}
}

func TestGenerateOutputContract(t *testing.T) {
	fixtures := filepath.Join("..", "..", "internal", "mermaidcode", "testdata")
	source := filepath.Join(fixtures, "sample.ts")
	output := filepath.Join(t.TempDir(), "user.class.mmd")
	if code := run([]string{"generate", "class", source, "User", "--source", source, "--output", output}); code != 0 {
		t.Fatalf("generate returned %d", code)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Fatal("generated output is empty")
	}
	structureOutput := filepath.Join(t.TempDir(), "user.structure.mmd")
	if code := run([]string{"generate", "structure", source, "User", "--source", source, "--output", structureOutput}); code != 0 {
		t.Fatalf("structure alias generation returned %d", code)
	}
}
