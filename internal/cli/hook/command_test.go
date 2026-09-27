package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func testHookRepository(t *testing.T) string {
	t.Helper()
	sourceDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("GREPPLE_CACHE_DIR", t.TempDir())
	configDir := filepath.Join(root, ".grepple", "hooks")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go-empty-if", "go-direct-dot-import"} {
		content, err := os.ReadFile(filepath.Join(sourceDirectory, "..", "..", "..", ".grepple", "hooks", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, name+".yaml"), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeHookTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitHookTest(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.test", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.test")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func runHookTest(t *testing.T, args ...string) (Report, int, error) {
	t.Helper()
	var output bytes.Buffer
	status := 0
	application := cliruntime.Environment{
		Output: &output, Exit: func(code int) { status = code },
		RepositoryContext: cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, nil),
	}
	err := New(application).Run(append(args, "--json"))
	if err != nil {
		return Report{}, status, err
	}
	var report Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON %q: %v", output.String(), err)
	}
	return report, status, nil
}

func TestHookChangedAllAndRepeatableIDs(t *testing.T) {
	root := testHookRepository(t)
	gitHookTest(t, root, "init", "-q")
	writeHookTestFile(t, root, "baseline.go", "package demo\nfunc baseline(ok bool) { if ok {} }\n")
	writeHookTestFile(t, root, "modified.go", "package demo\nfunc modified() {}\n")
	writeHookTestFile(t, root, "removed.go", "package demo\n")
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "baseline")
	writeHookTestFile(t, root, "modified.go", "package demo\nfunc modified(ok bool) { if ok {} }\n")
	writeHookTestFile(t, root, "staged.go", "package demo\nimport . \"fmt\"\n")
	gitHookTest(t, root, "add", "staged.go")
	writeHookTestFile(t, root, "new file.go", "package demo\nfunc untracked(ok bool) { if ok {} }\n")
	if err := os.Remove(filepath.Join(root, "removed.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("modified.go", filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	report, status, err := runHookTest(t, "--id", "go-empty-if", "--id", "go-direct-dot-import")
	if err != nil || status != 1 || report.Mode != "changed" || report.Files != 3 || len(report.Findings) != 3 {
		t.Fatalf("changed: report=%+v status=%d err=%v", report, status, err)
	}
	paths := []string{}
	for _, finding := range report.Findings {
		paths = append(paths, finding.Path)
	}
	if !reflect.DeepEqual(paths, []string{"modified.go", "new file.go", "staged.go"}) {
		t.Fatalf("changed paths=%v", paths)
	}
	selected, selectedStatus, err := runHookTest(t, "--id", "go-direct-dot-import")
	if err != nil || selectedStatus != 1 || len(selected.Hooks) != 1 || len(selected.Findings) != 1 || selected.Findings[0].Path != "staged.go" {
		t.Fatalf("selected: report=%+v status=%d err=%v", selected, selectedStatus, err)
	}
	all, allStatus, err := runHookTest(t, "--all")
	if err != nil || allStatus != 1 || all.Mode != "all" || len(all.Findings) != 4 || all.Findings[0].Path != "baseline.go" {
		t.Fatalf("all: report=%+v status=%d err=%v", all, allStatus, err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "nested"))
	nested, nestedStatus, err := runHookTest(t, "--id", "go-direct-dot-import")
	if err != nil || nestedStatus != 1 || len(nested.Findings) != 1 || nested.Findings[0].Path != "staged.go" {
		t.Fatalf("nested: report=%+v status=%d err=%v", nested, nestedStatus, err)
	}
	nestedAll, nestedAllStatus, err := runHookTest(t, "--all", "--id", "go-empty-if")
	if err != nil || nestedAllStatus != 1 || len(nestedAll.Findings) != 3 || nestedAll.Findings[0].Path != "baseline.go" {
		t.Fatalf("nested all: report=%+v status=%d err=%v", nestedAll, nestedAllStatus, err)
	}
}

func TestHookNoGitAllAndNoMatches(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, "ok.go", "package demo\nfunc fine(ok bool) { if ok { _ = ok } }\n")
	if _, _, err := runHookTest(t); err == nil || !strings.Contains(err.Error(), "--all") {
		t.Fatalf("default without Git error=%v", err)
	}
	report, status, err := runHookTest(t, "--all", "--id", "go-empty-if")
	if err != nil || status != 0 || report.Files != 1 || len(report.Findings) != 0 {
		t.Fatalf("all non-Git: report=%+v status=%d err=%v", report, status, err)
	}
}

func TestHookReportsSourceErrorsInsteadOfClean(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, "broken.go", "package demo\nfunc run(ok bool) { if ok {\n")
	if _, _, err := runHookTest(t, "--all", "--id", "go-empty-if"); err == nil || !strings.Contains(err.Error(), "SOURCE_PARSE") {
		t.Fatalf("invalid source error=%v", err)
	}
}

func TestHookCacheInvalidatesOnSourceAndConfigChanges(t *testing.T) {
	root := testHookRepository(t)
	original := "package demo\nfunc f(ok bool) { if ok {}   }\n"
	writeHookTestFile(t, root, "case.go", original)
	for run := 0; run < 2; run++ {
		report, status, err := runHookTest(t, "--all", "--id", "go-empty-if")
		if err != nil || status != 1 || len(report.Findings) != 1 {
			t.Fatalf("initial run %d: report=%+v status=%d err=%v", run, report, status, err)
		}
	}
	path := filepath.Join(root, "case.go")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(original, "if ok {}   ", "if ok {x}  ", 1)
	if len(changed) != len(original) {
		t.Fatalf("same-size fixture: old=%d new=%d", len(original), len(changed))
	}
	writeHookTestFile(t, root, "case.go", changed)
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if report, status, err := runHookTest(t, "--all", "--id", "go-empty-if"); err != nil || status != 0 || len(report.Findings) != 0 {
		t.Fatalf("changed bytes: report=%+v status=%d err=%v", report, status, err)
	}
	writeHookTestFile(t, root, "case.go", original)
	configPath := filepath.Join(root, ".grepple", "hooks", "go-empty-if.yaml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config = bytes.Replace(config, []byte("Remove the empty block"), []byte("Review the empty block"), 1)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
	report, status, err := runHookTest(t, "--all", "--id", "go-empty-if")
	if err != nil || status != 1 || len(report.Findings) != 1 || !strings.Contains(report.Findings[0].Message, "Review") {
		t.Fatalf("changed config: report=%+v status=%d err=%v", report, status, err)
	}
}

func TestHookConfigErrorsAndMissingIDs(t *testing.T) {
	root := testHookRepository(t)
	for _, id := range []string{"missing", "Bad"} {
		if _, _, err := runHookTest(t, "--all", "--id", id); err == nil || !strings.Contains(err.Error(), id) {
			t.Fatalf("id=%q error=%v", id, err)
		}
	}
	path := filepath.Join(root, ".grepple", "hooks", "go-empty-if.yaml")
	writeHookTestFile(t, root, ".grepple/hooks/go-empty-if.yaml", "id: go-empty-if\nunexpected: true\n")
	if _, _, err := runHookTest(t, "--all", "--id", "go-empty-if"); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unknown field error=%v", err)
	}
	if _, _, err := runHookTest(t, "--all", "--id", "go-direct-dot-import"); err != nil {
		t.Fatalf("selected healthy hook should skip malformed unselected config: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
