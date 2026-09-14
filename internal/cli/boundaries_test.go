package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundariesReportsExternalOwnerWorkflowsAndUsesCache(t *testing.T) {
	dir := chdirTemp(t)
	writeBoundaryFixture(t, dir)
	first := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		"boundary analysis paths=. files=3 workflow-candidates=1 workflow-shown=1 type-candidates=0 type-shown=0", "owner: owner.go [go, risk=low]", "reasons: repeated-owner-file-workflow, broad-owner-callable-surface", "external consumers: 2 functions / 2 files",
		"Parse + Validate", "Parse -> Validate", "callers: one.go:2 One; two.go:2 Two",
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("boundary output missing %q:\n%s", expected, first)
		}
	}
	second := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if second != first {
		t.Fatalf("cache changed deterministic output:\nfirst=%s\nsecond=%s", first, second)
	}
	if _, cache, err := buildCachedBoundaryGraph([]string{"."}, 0, true); err != nil || cache != "hit" {
		t.Fatalf("cache state=%q err=%v", cache, err)
	}
	entries, err := os.ReadDir(filepath.Join(".grepple", "cache", "boundaries"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache entries=%d err=%v", len(entries), err)
	}
}

func TestBoundariesHumanLimitDoesNotLimitJSON(t *testing.T) {
	dir := chdirTemp(t)
	writeBoundaryFixture(t, dir)
	writeGraphSource(t, dir, "other.go", "package sample\nfunc Read(){}\nfunc Write(){}\n")
	writeGraphSource(t, dir, "other_one.go", "package sample\nfunc OtherOne(){ Read(); Write() }\n")
	writeGraphSource(t, dir, "other_two.go", "package sample\nfunc OtherTwo(){ Read(); Write() }\n")
	limited := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "--no-cache", "--limit", "1", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(limited, "workflow-candidates=2 workflow-shown=1 type-candidates=0 type-shown=0") || !strings.Contains(limited, "omitted 1 lower-ranked workflow candidates") {
		t.Fatalf("limited output is misleading:\n%s", limited)
	}
	jsonText := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "--json", "--no-cache", "--limit", "1", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var output boundariesOutput
	if err := json.Unmarshal([]byte(jsonText), &output); err != nil {
		t.Fatal(err)
	}
	if output.Schema != "grepple-boundaries-v2" || len(output.Candidates) != 2 || output.Candidates[0].Risk == "" || len(output.Candidates[0].Reasons) == 0 {
		t.Fatalf("JSON was limited: %#v", output)
	}
	if _, err := os.Stat(filepath.Join(".grepple", "cache")); !os.IsNotExist(err) {
		t.Fatalf("--no-cache created cache: %v", err)
	}
}

func TestBoundariesReportsImportedTypeSpreadAndPublicExposure(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "one.go", `package sample
import sitter "github.com/tree-sitter/go-tree-sitter"
func Public(node *sitter.Node) {}
`)
	writeGraphSource(t, dir, "two.go", `package sample
import sitter "github.com/tree-sitter/go-tree-sitter"
func private(node *sitter.Node) {}
`)
	output := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "--no-cache", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"type-candidates=1", "github.com/tree-sitter/go-tree-sitter.Node", "origin=third-party, risk=critical", "reasons: third-party-public-api", "2 production files / 0 test files", "1 public third-party exposures", "one.go:3 Public (parameter)"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("type boundary output missing %q:\n%s", expected, output)
		}
	}
}

func TestBoundaryCacheInvalidatesWhenSourceChanges(t *testing.T) {
	dir := chdirTemp(t)
	writeBoundaryFixture(t, dir)
	captureStdout(t, func() {
		if err := Run([]string{"boundaries", "."}); err != nil {
			t.Fatal(err)
		}
	})
	path := filepath.Join(dir, "two.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Two(){ Parse(); Validate(); Parse() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, cache, err := buildCachedBoundaryGraph([]string{"."}, 0, true); err != nil || cache != "miss" {
		t.Fatalf("cache state=%q err=%v", cache, err)
	}
}

func writeBoundaryFixture(t *testing.T, dir string) {
	t.Helper()
	writeGraphSource(t, dir, "owner.go", "package sample\nfunc Parse(){}\nfunc Validate(){}\n")
	writeGraphSource(t, dir, "one.go", "package sample\nfunc One(){ Parse(); Validate() }\n")
	writeGraphSource(t, dir, "two.go", "package sample\nfunc Two(){ Parse(); Validate() }\n")
}
