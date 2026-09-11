package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGrammarFingerprintChangesWithGeneratedSources(t *testing.T) {
	directory := t.TempDir()
	writeGeneratorFixture(t, directory, "parser.c", "parser-one")
	writeGeneratorFixture(t, directory, "node-types.json", "[]")
	first, err := grammarFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	writeGeneratorFixture(t, directory, "parser.c", "parser-two")
	second, err := grammarFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("parser source change did not invalidate fingerprint %q", first)
	}
	writeGeneratorFixture(t, directory, "scanner.c", "scanner-one")
	third, err := grammarFingerprint(directory)
	if err != nil {
		t.Fatal(err)
	}
	if second == third {
		t.Fatalf("scanner source change did not invalidate fingerprint %q", second)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	metadata := map[string]grammarMetadata{
		"go": {
			Fingerprint: "sha256:test",
			Fields: map[string]map[string]bool{
				"z": {"many": true, "one": false},
				"a": {"value": false},
			},
			Children: map[string]bool{"z": true, "a": false},
		},
	}
	first, err := render(metadata)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("metadata rendering is not deterministic")
	}
}

func writeGeneratorFixture(t *testing.T, directory, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
