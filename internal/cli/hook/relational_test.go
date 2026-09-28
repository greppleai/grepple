package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const duplicateHookYAML = `version: 1
id: duplicate-symbol
engine: gritql-relational-v1
include:
  - "**/*.ts"
severity: warning
message: "duplicate {{key}} also appears in {{left.basename}}"
relation:
  left_query: |
    language typescript
    variable_declarator(name=$name)
  left_key:
    binding: name
  right_key:
    binding: name
  scope: repository
`

func TestHookGenericCrossFileDuplicatesRequireFullSnapshot(t *testing.T) {
	root := testHookRepository(t)
	gitHookTest(t, root, "init", "-q")
	writeHookTestFile(t, root, "a/one.ts", "const shared = 1;\n")
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "first declaration")
	writeHookTestFile(t, root, "b/two.ts", "const shared = 2;\n")
	writeHookTestFile(t, root, ".grepple/hooks/duplicate-symbol.yaml", duplicateHookYAML)
	report, status, err := runHookTest(t, "--id", "duplicate-symbol")
	if err != nil || status != 1 || report.Mode != "all" || len(report.Findings) != 2 {
		t.Fatalf("report=%+v status=%d err=%v", report, status, err)
	}
	if report.Findings[0].Path != "a/one.ts" || report.Findings[1].Path != "b/two.ts" || !strings.Contains(report.Findings[1].Message, "one.ts") {
		t.Fatalf("findings=%v", report.Findings)
	}
	writeHookTestFile(t, root, "b/two.ts", "const unique = 2;\n")
	changed, status, err := runHookTest(t, "--id", "duplicate-symbol")
	if err != nil || status != 0 || len(changed.Findings) != 0 {
		t.Fatalf("changed source reused stale result: %+v status=%d err=%v", changed, status, err)
	}
	writeHookTestFile(t, root, "b/two.ts", "const shared = 2;\n")
	if restored, code, err := runHookTest(t, "--id", "duplicate-symbol"); err != nil || code != 1 || len(restored.Findings) != 2 {
		t.Fatalf("restored source: %+v status=%d err=%v", restored, code, err)
	}
	if err := os.Remove(filepath.Join(root, "b/two.ts")); err != nil {
		t.Fatal(err)
	}
	clean, status, err := runHookTest(t, "--id", "duplicate-symbol")
	if err != nil || status != 0 || len(clean.Findings) != 0 {
		t.Fatalf("after removal: %+v status=%d err=%v", clean, status, err)
	}
}

func TestHookRelationalInvalidConfigAndSourceFailClosed(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/duplicate-symbol.yaml", strings.Replace(duplicateHookYAML, "{{key}}", "{{unknown}}", 1))
	if _, _, err := runHookTest(t, "--id", "duplicate-symbol"); err == nil || !strings.Contains(err.Error(), "unknown relation message") {
		t.Fatalf("invalid config error=%v", err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/duplicate-symbol.yaml", duplicateHookYAML)
	writeHookTestFile(t, root, "bad.ts", "const broken = ;\n")
	if _, _, err := runHookTest(t, "--id", "duplicate-symbol"); err == nil || !strings.Contains(err.Error(), "SOURCE_PARSE") {
		t.Fatalf("invalid source error=%v", err)
	}
}

func TestHookDefaultMixesLocalAndRelationalRules(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/duplicate-symbol.yaml", duplicateHookYAML)
	writeHookTestFile(t, root, "a.ts", "const shared = 1;\n")
	writeHookTestFile(t, root, "b.ts", "const shared = 2;\n")
	writeHookTestFile(t, root, "thing.go", "package demo\nfunc run(ok bool) { if ok {} }\n")
	report, status, err := runHookTest(t)
	if err != nil || status != 1 || report.Mode != "all" || len(report.Findings) != 3 || len(report.Hooks) != 3 {
		t.Fatalf("mixed hooks report=%+v status=%d err=%v", report, status, err)
	}
	for index, want := range []string{"a.ts", "b.ts", "thing.go"} {
		if report.Findings[index].Path != want {
			t.Fatalf("findings=%v", report.Findings)
		}
	}
}
