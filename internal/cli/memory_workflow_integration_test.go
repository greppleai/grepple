package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/memory"
	"github.com/greppleai/grepple/internal/wire"
)

func memoryWorkflowFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func memoryWorkflowRun(t *testing.T, args ...string) string {
	t.Helper()
	return captureStdout(t, func() {
		if err := Run(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	})
}
func memoryWorkflowMetadata(t *testing.T, root, digest string) {
	t.Helper()
	doc := directorymeta.DocumentV1{Schema: "v1", Directory: map[string]directorymeta.DirectoryV1{".": {Files: []directorymeta.FileV1{{Name: "a.go", Checksum: digest, Description: "Application code", Purpose: "implementation", DomainAreas: []string{"parser"}, FunctionalAreas: []string{"search"}, Keywords: []string{"needle"}}}}}}
	if err := directorymeta.WriteDocumentV1(root, doc); err != nil {
		t.Fatal(err)
	}
}
func TestMemoryEndToEndCRUDSearchRecallAndStaleRefresh(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".git", 0700); err != nil {
		t.Fatal(err)
	}
	source := "package a\nfunc Alpha() {\n // needle\n}\nfunc Beta() { // needle\n}\n"
	memoryWorkflowFile(t, "a.go", source)
	memoryWorkflowFile(t, "body.txt", "Private markdown content\n")
	_, checksum, err := memory.Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	memoryWorkflowMetadata(t, root, checksum)
	for _, spec := range [][]string{
		{"symbol", "--symbol", "a.go@func:2-4"},
		{"range", "--line-range", "a.go:3-3"},
		{"whole", "--file", "a.go"},
		{"label", "--domain-area", "parser", "--functional-area", "search", "--keyword", "needle"},
	} {
		args := append([]string{"memory", "store", spec[0], "--description", "Short note", "--body-file", "body.txt"}, spec[1:]...)
		memoryWorkflowRun(t, args...)
	}
	list := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "a.go")
	for _, wanted := range []string{"Memories located:", "recall symbol", "recall range", "recall whole", "1 more memories"} {
		if !strings.Contains(list, wanted) {
			t.Fatalf("missing %q in %q", wanted, list)
		}
	}
	if strings.Contains(list, "recall label") || strings.Contains(list, "Private markdown content") {
		t.Fatalf("unbounded/private output %q", list)
	}
	for _, mode := range [][]string{{"-o", "-F", "Beta", "a.go"}, {"-C", "1", "-F", "Beta", "a.go"}} {
		result := memoryWorkflowRun(t, append([]string{"search"}, mode...)...)
		if strings.Contains(result, "recall symbol") || strings.Contains(result, "recall range") || !strings.Contains(result, "recall whole") || !strings.Contains(result, "recall label") {
			t.Fatalf("context or only-matching widened evidence %v: %q", mode, result)
		}
	}
	var page wire.SearchResponse
	if err := json.Unmarshal([]byte(memoryWorkflowRun(t, "search", "--json", "-F", "needle", "a.go")), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Memories) != 3 || page.MemoryStatus == nil || !page.MemoryStatus.Complete || page.MemoryStatus.Omitted != 1 {
		t.Fatalf("JSON references %+v %+v", page.Memories, page.MemoryStatus)
	}
	onlyBeta := memoryWorkflowRun(t, "search", "--line-only", "-F", "Beta", "a.go")
	if strings.Contains(onlyBeta, "recall symbol") || strings.Contains(onlyBeta, "recall range") || !strings.Contains(onlyBeta, "recall whole") || !strings.Contains(onlyBeta, "recall label") {
		t.Fatalf("match-line scope %q", onlyBeta)
	}
	var staleStatus string
	recalled := captureStdout(t, func() {
		staleStatus = captureStderr(t, func() {
			if err := Run([]string{"memory", "recall", "symbol"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	if !strings.Contains(recalled, "Private markdown content") || !strings.Contains(staleStatus, "current") {
		t.Fatalf("recall body/status %q / %q", recalled, staleStatus)
	}
	// Insert a line before both functions: content still matches, old checksums
	// and file-owned metadata become stale. Nothing is rewritten on search.
	memoryWorkflowFile(t, "a.go", "// inserted\n"+source)
	if after := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "a.go"); strings.Contains(after, "Memories located:") || strings.Contains(after, "recall symbol") {
		t.Fatalf("stale source was claimed current: %q", after)
	}
	staleStatus = captureStderr(t, func() {
		if err := Run([]string{"memory", "recall", "symbol"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(staleStatus, "stale checksum") {
		t.Fatalf("missing stale warning: %q", staleStatus)
	}
	// A copied, now-wrong selector must fail, even in an explicit update.
	if err := Run([]string{"memory", "update", "symbol", "--description", "Updated", "--body-file", "body.txt", "--symbol", "a.go@func:2-4"}); err == nil {
		t.Fatal("wrong selector accepted")
	}
	memoryWorkflowRun(t, "memory", "update", "symbol", "--description", "Updated", "--body-file", "body.txt", "--symbol", "a.go@func:3-5")
	memoryWorkflowRun(t, "memory", "update", "range", "--description", "Updated", "--body-file", "body.txt", "--line-range", "a.go:4-4")
	if refreshed := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "a.go"); !strings.Contains(refreshed, "recall symbol") || !strings.Contains(refreshed, "recall range") || strings.Contains(refreshed, "recall whole") || strings.Contains(refreshed, "recall label") {
		t.Fatalf("refresh widened stale entries: %q", refreshed)
	}
	_, checksum, err = memory.Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	memoryWorkflowMetadata(t, root, checksum)
	if labeled := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "a.go"); !strings.Contains(labeled, "recall label") {
		t.Fatalf("current labels absent: %q", labeled)
	}
	memoryWorkflowRun(t, "memory", "delete", "symbol")
	if deleted := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "a.go"); strings.Contains(deleted, "recall symbol") {
		t.Fatalf("deleted entry surfaced: %q", deleted)
	}
	if err := Run([]string{"memory", "recall", "symbol"}); err == nil {
		t.Fatal("deleted entry recalled")
	}
}

func TestMemoryEndToEndUnsafeEntriesFailClosed(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".git", 0700); err != nil {
		t.Fatal(err)
	}
	memoryWorkflowFile(t, "a.go", "package a\nfunc A() { // needle\n}\n")
	memoryWorkflowFile(t, "body.txt", "Full private body\n")
	memoryWorkflowRun(t, "memory", "store", "valid", "--description", "Short note", "--body-file", "body.txt", "--file", "a.go")
	dir := filepath.Join(root, ".grepple", "memories")
	memoryWorkflowFile(t, filepath.Join(dir, "invalid.md"), "---\nunknown: true\n---\nbody")
	if err := os.Symlink(filepath.Join(dir, "valid.md"), filepath.Join(dir, "linked.md")); err != nil {
		t.Fatal(err)
	}
	result := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "a.go")
	if !strings.Contains(result, "recall valid") || !strings.Contains(result, "memory references incomplete") || strings.Contains(result, "recall linked") {
		t.Fatalf("unsafe entries %q", result)
	}
	for _, name := range []string{"invalid", "linked"} {
		if err := Run([]string{"memory", "recall", name}); err == nil {
			t.Fatalf("unsafe recall %q accepted", name)
		}
	}
	// A symlinked entire memory directory also fails closed, including writes.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, dir); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"memory", "store", "unsafe", "--description", "Short note", "--body-file", "body.txt", "--file", "a.go"}); err == nil {
		t.Fatal("store followed symlink directory")
	}
	if output := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "a.go"); !strings.Contains(output, "memory references incomplete") || strings.Contains(output, "recall valid") {
		t.Fatalf("symlink directory leaked: %q", output)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside directory touched: %v %+v", err, entries)
	}
}

func TestMemoryScopeSearchUsesOnlySelectedLocalMatches(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".git", 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"internal/cli/direct.go":      "package cli\nfunc Direct() { // needle\n}\n",
		"internal/cli/nested/deep.go": "package nested\nfunc Deep() { // needle\n}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		memoryWorkflowFile(t, name, content)
	}
	memoryWorkflowFile(t, "body.txt", "Private scope body\n")
	for _, spec := range [][]string{
		{"directory", "internal/cli/"},
		{"direct", "internal/cli/*.go"},
		{"recursive", "internal/cli/**/*.go"},
	} {
		memoryWorkflowRun(t, "memory", "store", spec[0], "--description", "Scope note", "--body-file", "body.txt", "--scope", spec[1])
	}
	direct := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "internal/cli/direct.go")
	for _, name := range []string{"recall directory", "recall direct", "recall recursive"} {
		if !strings.Contains(direct, name) {
			t.Fatalf("direct result missing %q: %q", name, direct)
		}
	}
	if strings.Contains(direct, "Private scope body") {
		t.Fatalf("scope search exposed body: %q", direct)
	}
	var full wire.SearchResponse
	if err := json.Unmarshal([]byte(memoryWorkflowRun(t, "search", "--json", "-F", "needle", "internal/cli/direct.go")), &full); err != nil || len(full.Memories) != 3 {
		t.Fatalf("scope JSON response: %v %+v", err, full)
	}
	var matches map[string]json.RawMessage
	if err := json.Unmarshal([]byte(memoryWorkflowRun(t, "search", "--json-matches", "-F", "needle", "internal/cli/direct.go")), &matches); err != nil || matches["memories"] == nil || matches["memoryStatus"] == nil {
		t.Fatalf("scope JSON-matches response: %v %q", err, matches)
	}
	recalled := memoryWorkflowRun(t, "memory", "recall", "directory")
	if !strings.Contains(recalled, "scopes:") || !strings.Contains(recalled, "internal/cli/") {
		t.Fatalf("scope recall %q", recalled)
	}
	nested := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "internal/cli/nested/deep.go")
	if !strings.Contains(nested, "recall directory") || !strings.Contains(nested, "recall recursive") || strings.Contains(nested, "recall direct)") {
		t.Fatalf("nested scope result %q", nested)
	}
	memoryWorkflowFile(t, "internal/cli/future.go", "package cli\nfunc Future() { // needle\n}\n")
	future := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "internal/cli/future.go")
	if !strings.Contains(future, "recall directory") || !strings.Contains(future, "recall direct") || !strings.Contains(future, "recall recursive") {
		t.Fatalf("future scoped path %q", future)
	}
	if err := os.MkdirAll("internal/other", 0700); err != nil {
		t.Fatal(err)
	}
	memoryWorkflowRun(t, "memory", "update", "direct", "--description", "Replacement scope", "--body-file", "body.txt", "--scope", "internal/other/")
	if updated := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "internal/cli/direct.go"); strings.Contains(updated, "recall direct)") {
		t.Fatalf("full update retained old scope: %q", updated)
	}
	memoryWorkflowRun(t, "memory", "delete", "recursive")
	if deleted := memoryWorkflowRun(t, "search", "--line-only", "-F", "needle", "internal/cli/nested/deep.go"); strings.Contains(deleted, "recall recursive") {
		t.Fatalf("deleted scope remained discoverable: %q", deleted)
	}
	if outline := memoryWorkflowRun(t, "search", "--outline", "-F", "needle", "internal/cli/direct.go"); strings.Contains(outline, "Memories located:") {
		t.Fatalf("outline mode exposed memory scopes: %q", outline)
	}
}
