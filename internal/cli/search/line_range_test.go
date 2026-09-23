package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLineOnlyReportsAnchoredLocations(t *testing.T) {
	path := writeLineRangeFixture(t)
	functionOutput := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "func Run", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(functionOutput, "sample.go") || !strings.Contains(functionOutput, "│2│func Run() {") {
		t.Fatalf("anchored function location missing: %s", functionOutput)
	}

	branchOutput := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "if ready", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(branchOutput, "sample.go") || !strings.Contains(branchOutput, "│3│\tif ready {") {
		t.Fatalf("anchored branch location missing: %s", branchOutput)
	}

	plainOutput := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "const plain", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(plainOutput, "sample.go") || !strings.Contains(plainOutput, "│7│const plain = 1") {
		t.Fatalf("anchored single-line location missing: %s", plainOutput)
	}
}

func TestLineOnlyEnclosingReportsNearestBodyScope(t *testing.T) {
	path := writeLineRangeFixture(t)
	bodyOutput := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "work()", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(bodyOutput, "sample.go") || !strings.Contains(bodyOutput, "│4│\t\twork()") {
		t.Fatalf("anchored body location missing: %s", bodyOutput)
	}

	enclosingOutput := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "--enclosing", "-F", "work()", path}); err != nil {
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
		if err := runTestSearch([]string{"--line-only", "--enclosing", "--json", "-F", "work()", path}); err != nil {
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
