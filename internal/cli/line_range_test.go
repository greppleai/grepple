package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLineOnlyReportsConstructEndRange(t *testing.T) {
	root := t.TempDir()
	content := "package sample\nfunc Run() {\n\tif ready {\n\t\twork()\n\t}\n}\nconst plain = 1\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	functionOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--no-anchors", "-F", "func Run", filepath.Join(root, "sample.go")}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(functionOutput, "sample.go:2-6:func Run() {") {
		t.Fatalf("function range missing: %s", functionOutput)
	}

	branchOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--no-anchors", "-F", "if ready", filepath.Join(root, "sample.go")}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(branchOutput, "sample.go:3-5:\tif ready {") {
		t.Fatalf("branch range missing: %s", branchOutput)
	}

	plainOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--no-anchors", "-F", "const plain", filepath.Join(root, "sample.go")}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(plainOutput, "sample.go:7:const plain = 1") {
		t.Fatalf("single-line location changed: %s", plainOutput)
	}

	jsonOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--json", "-F", "func Run", filepath.Join(root, "sample.go")}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(jsonOutput, `"line": 2`) || !strings.Contains(jsonOutput, `"endLine": 6`) {
		t.Fatalf("JSON construct range missing: %s", jsonOutput)
	}
}
