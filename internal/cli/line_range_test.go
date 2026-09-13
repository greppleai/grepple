package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLineOnlyReportsConstructEndRange(t *testing.T) {
	path := writeLineRangeFixture(t)
	functionOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--no-anchors", "-F", "func Run", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(functionOutput, "sample.go:2-6:func Run() {") {
		t.Fatalf("function range missing: %s", functionOutput)
	}

	branchOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--no-anchors", "-F", "if ready", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(branchOutput, "sample.go:3-5:\tif ready {") {
		t.Fatalf("branch range missing: %s", branchOutput)
	}

	plainOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--no-anchors", "-F", "const plain", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(plainOutput, "sample.go:7:const plain = 1") {
		t.Fatalf("single-line location changed: %s", plainOutput)
	}
}

func TestLineOnlyEnclosingReportsNearestBodyScope(t *testing.T) {
	path := writeLineRangeFixture(t)
	bodyOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--no-anchors", "-F", "work()", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(bodyOutput, "sample.go:4:\t\twork()") {
		t.Fatalf("default body location changed: %s", bodyOutput)
	}

	enclosingOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--enclosing", "-F", "work()", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(enclosingOutput, "sample.go:4@3-5:\t\twork()") {
		t.Fatalf("enclosing branch range missing: %s", enclosingOutput)
	}
}

func TestLineOnlyEnclosingJSONReportsMatchAndScope(t *testing.T) {
	path := writeLineRangeFixture(t)
	output := captureStdout(t, func() {
		if err := runSearch([]string{"--line-only", "--enclosing", "--json", "-F", "work()", path}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{`"line": 4`, `"startLine": 3`, `"endLine": 5`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("JSON enclosing range missing %s: %s", expected, output)
		}
	}
}

func writeLineRangeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	content := "package sample\nfunc Run() {\n\tif ready {\n\t\twork()\n\t}\n}\nconst plain = 1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
