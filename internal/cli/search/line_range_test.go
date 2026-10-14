package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/hashline"
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

// Repeated reads must not depend on earlier source output, including after edits.
func TestRepeatedFocusedHumanReadsReturnFullAnchoredSource(t *testing.T) {
	path := writeLineRangeFixture(t)
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "absent-settings.json"))
	read := func() string {
		t.Helper()
		return captureStdout(t, func() {
			if err := runTestSearch([]string{"--at", path + ":1-7", "--line-only"}); err != nil {
				t.Fatal(err)
			}
		})
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	first := read()
	oldAnchor := hashline.Lines(string(original))[6]
	if !strings.Contains(first, oldAnchor+"│7│const plain = 1") {
		t.Fatalf("read did not emit expected edit-ready hashline anchor %q: %s", oldAnchor, first)
	}
	if second := read(); second != first {
		t.Fatalf("identical focused reads differ:\nfirst: %s\nsecond: %s", first, second)
	}
	for _, text := range []string{"│1│package sample", "│4│\t\twork()", "│7│const plain = 1"} {
		if !strings.Contains(first, text) {
			t.Fatalf("focused read omitted %q: %s", text, first)
		}
	}
	updated := "package sample\nfunc Run() {\n\tif ready {\n\t\twork()\n\t}\n}\nconst plain = 2\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := read()
	newAnchor := hashline.Lines(updated)[6]
	if newAnchor == oldAnchor || !strings.Contains(changed, newAnchor+"│7│const plain = 2") {
		t.Fatalf("edited read did not emit fresh hashline anchor %q: %s", newAnchor, changed)
	}
	if strings.Contains(changed, "│7│const plain = 1") || !strings.Contains(changed, "│7│const plain = 2") || changed == first {
		t.Fatalf("edited read did not return fresh source: %s", changed)
	}
}

func TestRepeatedFocusedJSONReadsReturnCurrentSource(t *testing.T) {
	path := writeLineRangeFixture(t)
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "absent-settings.json"))
	read := func() string {
		t.Helper()
		return captureStdout(t, func() {
			if err := runTestSearch([]string{"--at", path + ":1-7", "--line-only", "--json"}); err != nil {
				t.Fatal(err)
			}
		})
	}
	first := read()
	if second := read(); second != first || !strings.Contains(second, "const plain = 1") {
		t.Fatalf("repeated JSON omitted or changed source: %s vs %s", first, second)
	}
	updated := "package sample\nfunc Run() {\n\tif ready {\n\t\twork()\n\t}\n}\nconst plain = 2\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed := read(); changed == first || !strings.Contains(changed, "const plain = 2") {
		t.Fatalf("edited JSON read omitted current source: %s", changed)
	}
}

func TestRepeatedStructuralReadsKeepCompleteSource(t *testing.T) {
	path := writeLineRangeFixture(t)
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "absent-settings.json"))
	read := func() string {
		t.Helper()
		return captureStdout(t, func() {
			if err := runTestSearch([]string{"-F", "func Run", path}); err != nil {
				t.Fatal(err)
			}
		})
	}
	first := read()
	if second := read(); second != first {
		t.Fatalf("structural source changed across repeated reads:\n%s\n%s", first, second)
	}
	for _, text := range []string{"func Run() {", "if ready {", "work()"} {
		if !strings.Contains(first, text) {
			t.Fatalf("structural source omitted %q: %s", text, first)
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
