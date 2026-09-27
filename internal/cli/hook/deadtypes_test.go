package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeadTypeRulesRespectPackageAndRepositoryConsumers(t *testing.T) {
	sourceDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"go-dead-private-types", "go-dead-internal-types", "go-dead-internal-methods"}
	rules := make(map[string]string, len(names))
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(sourceDirectory, "..", "..", "..", ".grepple", "hooks", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		rules[name] = string(content)
	}
	root := testHookRepository(t)
	gitHookTest(t, root, "init", "-q")
	for _, name := range names {
		writeHookTestFile(t, root, ".grepple/hooks/"+name+".yaml", rules[name])
	}
	writeHookTestFile(t, root, "internal/lib/types.go", `package lib
 type hidden struct{}
 type usedHidden struct{}
 var _ usedHidden
 type ExternalUsed struct{}
 type Unused struct{}
 type MethodOwner struct{}
 func (MethodOwner) Called() {}
 func (MethodOwner) Uncalled() {}
 `)
	writeHookTestFile(t, root, "cmd/tool/main.go", `package main
 import "example.com/demo/internal/lib"
 var _ lib.ExternalUsed
 func call(owner lib.MethodOwner) { owner.Called() }
 `)
	writeHookTestFile(t, root, "internal/lib/types_test.go", `package lib
 var _ hidden
 var _ Unused
 func testOnly(owner MethodOwner) { owner.Uncalled() }
 `)
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "baseline")
	check := func(id string, expected ...string) {
		t.Helper()
		report, status, err := runHookTest(t, "--id", id)
		if err != nil || report.Mode != "all" || report.Files != 3 || len(report.Findings) != len(expected) || status != 1 {
			t.Fatalf("hook %s: report=%+v status=%d err=%v", id, report, status, err)
		}
		for index, name := range expected {
			finding := report.Findings[index]
			if finding.Path != "internal/lib/types.go" || finding.ID != id || !strings.Contains(finding.Message, name) {
				t.Fatalf("hook %s finding[%d]=%+v, want %s", id, index, finding, name)
			}
		}
	}
	check(names[0], "hidden")
	check(names[1], "Unused")
	check(names[2], "Uncalled")

	// A source outside internal/ changes only the reference universe; the
	// unchanged declaration must become a candidate and invalidate its cache.
	writeHookTestFile(t, root, "cmd/tool/main.go", "package main\nfunc call() {}\n")
	check(names[1], "ExternalUsed", "Unused")

	invalid := strings.Replace(rules[names[1]], "relation:\n", "relation:\n  max_findings: -1\n", 1)
	writeHookTestFile(t, root, ".grepple/hooks/"+names[1]+".yaml", invalid)
	if _, _, err := runHookTest(t, "--id", names[1]); err == nil || !strings.Contains(err.Error(), "max_findings") {
		t.Fatalf("invalid finding ceiling: %v", err)
	}
	bounded := strings.Replace(rules[names[0]], "max_findings: 50000", "max_findings: 1", 1)
	writeHookTestFile(t, root, ".grepple/hooks/"+names[0]+".yaml", bounded)
	if _, _, err := runHookTest(t, "--id", names[0]); err == nil || !strings.Contains(err.Error(), "LIMIT_FINDINGS") {
		t.Fatalf("exhausted finding ceiling must fail closed: %v", err)
	}
}
