package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookUnmatchedMethodsUseFullSnapshotAndInvalidateCache(t *testing.T) {
	sourceDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rule, err := os.ReadFile(filepath.Join(sourceDirectory, "..", "..", "..", "examples", "go-uncalled-methods.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	root := testHookRepository(t)
	gitHookTest(t, root, "init", "-q")
	writeHookTestFile(t, root, ".grepple/hooks/go-uncalled-methods.yaml", string(rule))
	writeHookTestFile(t, root, "pkg/defs.go", "package demo\ntype Target struct{}\nfunc (Target) unused() {}\nfunc (Target) used() {}\nfunc (Target) testOnly() {}\n")
	writeHookTestFile(t, root, "pkg/calls.go", "package demo\nfunc call(t Target) { t.used() }\n")
	writeHookTestFile(t, root, "pkg/calls_test.go", "package demo\nimport \"testing\"\nfunc (Target) onlyDeclaredInTests() {}\nfunc TestOnly(t *testing.T) { var target Target; target.testOnly() }\n")
	writeHookTestFile(t, root, "root_test.go", "package demo\nfunc broken( {\n") // root-level test source is excluded too
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "baseline")
	check := func(names ...string) {
		t.Helper()
		report, status, err := runHookTest(t, "--id", "go-uncalled-methods")
		if err != nil || status != 1 || report.Mode != "all" || report.Files != 4 || len(report.Findings) != len(names) {
			t.Fatalf("unmatched report=%+v status=%d err=%v", report, status, err)
		}
		for index, name := range names {
			finding := report.Findings[index]
			if finding.Path != "pkg/defs.go" || !strings.Contains(finding.Message, "method "+name+" has no matching selector") {
				t.Fatalf("finding[%d]=%+v, want method %q", index, finding, name)
			}
		}
	}
	check("unused", "testOnly") // a call in _test.go does not suppress testOnly
	check("unused", "testOnly") // content-validated cache hit
	writeHookTestFile(t, root, "pkg/calls_test.go", "package demo\nfunc broken( {\n")
	check("unused", "testOnly") // malformed excluded tests cannot invalidate the audit
	writeHookTestFile(t, root, "pkg/calls.go", "package demo\nfunc call(t Target) { _ = t }\n")
	check("unused", "used", "testOnly") // unchanged declaration becomes unreferenced

	writeHookTestFile(t, root, ".grepple/hooks/go-uncalled-methods.yaml", strings.Replace(string(rule), "{{left.name}}", "{{right.name}}", 1))
	if _, _, err := runHookTest(t, "--id", "go-uncalled-methods"); err == nil || !strings.Contains(err.Error(), "requires a matched right") {
		t.Fatalf("right-side placeholder for unmatched mode: %v", err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/go-uncalled-methods.yaml", string(rule))
	writeHookTestFile(t, root, "pkg/broken.go", "package demo\nfunc broken( {\n")
	if _, _, err := runHookTest(t, "--id", "go-uncalled-methods"); err == nil || !strings.Contains(err.Error(), "SOURCE_PARSE") {
		t.Fatalf("incomplete negative hook scan: %v", err)
	}
}
