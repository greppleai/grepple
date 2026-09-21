package boundaries

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	writecommand "github.com/greppleai/grepple/internal/cli/write"
	"github.com/greppleai/grepple/parser"
)

func TestBoundariesReportsExternalOwnerWorkflowsAndUsesCache(t *testing.T) {
	dir := chdirTemp(t)
	t.Setenv("GREPPLE_CACHE_DIR", filepath.Join(dir, ".grepple", "cache"))
	writeBoundaryFixture(t, dir)
	first := captureStdout(t, func() {
		if err := runBoundaries([]string{"."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		"boundary analysis paths=. files=3 workflow-candidates=1 workflow-shown=1 type-candidates=0 type-shown=0 facade-bypasses=0", "owner: owner.go [go, spread=package-internal, containment=approved, risk=low]", "reasons: repeated-owner-file-workflow, broad-owner-callable-surface", "external consumers: 2 functions / 2 files",
		"Parse + Validate", "Parse -> Validate", "callers: one.go:2 One; two.go:2 Two",
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("boundary output missing %q:\n%s", expected, first)
		}
	}
	second := captureStdout(t, func() {
		if err := runBoundaries([]string{"."}); err != nil {
			t.Fatal(err)
		}
	})
	if second != first {
		t.Fatalf("cache changed deterministic output:\nfirst=%s\nsecond=%s", first, second)
	}
	if _, cache, err := buildCachedBoundaryGraphForTest([]string{"."}, 0, true); err != nil || cache != "hit" {
		t.Fatalf("cache state=%q err=%v", cache, err)
	}
	entries, err := os.ReadDir(filepath.Join(".grepple", "cache", "boundaries"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache entries=%d err=%v", len(entries), err)
	}
}

func TestReadOnlyAnalysisStoresCachesAndArtifactsOutsideRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions differ on Windows")
	}
	dir := chdirTemp(t)
	writeBoundaryFixture(t, dir)
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("GREPPLE_CACHE_DIR", "")
	t.Setenv("GREPPLE_ARTIFACT_DIR", "")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	captureStdout(t, func() {
		if err := runBoundaries([]string{"."}); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, ".grepple")); !os.IsNotExist(err) {
		t.Fatalf("analysis wrote beneath read-only repository: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(cacheHome, "grepple", "cache", writecommand.Digest([]byte(filepath.Clean(dir)))[:16], "boundaries"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("external cache entries=%d err=%v", len(entries), err)
	}
}

func TestBoundariesHumanLimitDoesNotLimitJSON(t *testing.T) {
	dir := chdirTemp(t)
	t.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(dir, ".grepple", "cache", "navigation"))
	writeBoundaryFixture(t, dir)
	writeGraphSource(t, dir, "other.go", "package sample\nfunc Read(){}\nfunc Write(){}\n")
	writeGraphSource(t, dir, "other_one.go", "package sample\nfunc OtherOne(){ Read(); Write() }\n")
	writeGraphSource(t, dir, "other_two.go", "package sample\nfunc OtherTwo(){ Read(); Write() }\n")
	limited := captureStdout(t, func() {
		if err := runBoundaries([]string{"--no-cache", "--limit", "1", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(limited, "workflow-candidates=2 workflow-shown=1 type-candidates=0 type-shown=0") || !strings.Contains(limited, "omitted 1 lower-ranked workflow candidates") {
		t.Fatalf("limited output is misleading:\n%s", limited)
	}
	jsonText := captureStdout(t, func() {
		if err := runBoundaries([]string{"--json", "--no-cache", "--limit", "1", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var output boundariesOutput
	if err := json.Unmarshal([]byte(jsonText), &output); err != nil {
		t.Fatal(err)
	}
	if output.Schema != "grepple-boundaries-v3" || len(output.Candidates) != 2 || output.Candidates[0].Risk == "" || len(output.Candidates[0].Reasons) == 0 {
		t.Fatalf("JSON was limited: %#v", output)
	}
	if output.Metadata == nil || !output.Metadata.Page.Complete || output.Metadata.Page.Returned != 2 || output.Metadata.Scope.Paths[0] != "." {
		t.Fatalf("boundary metadata=%#v", output.Metadata)
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
		if err := runBoundaries([]string{"--no-cache", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"type-candidates=1", "github.com/tree-sitter/go-tree-sitter.Node", "origin=third-party, spread=public-api, containment=unknown, risk=critical", "reasons: third-party-public-api", "2 production files / 0 test files", "1 public third-party exposures", "one.go:3 Public (parameter)"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("type boundary output missing %q:\n%s", expected, output)
		}
	}
}

func TestBoundaryCacheInvalidatesWhenSourceChanges(t *testing.T) {
	dir := chdirTemp(t)
	writeBoundaryFixture(t, dir)
	captureStdout(t, func() {
		if err := runBoundaries([]string{"."}); err != nil {
			t.Fatal(err)
		}
	})
	path := filepath.Join(dir, "two.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Two(){ Parse(); Validate(); Parse() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, cache, err := buildCachedBoundaryGraphForTest([]string{"."}, 0, true); err != nil || cache != "miss" {
		t.Fatalf("cache state=%q err=%v", cache, err)
	}
}

func TestBoundaryCacheInvalidatesWhenRepositoryIdentityChanges(t *testing.T) {
	dir := chdirTemp(t)
	writeBoundaryFixture(t, dir)
	modulePath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(modulePath, []byte("module example.com/one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, cache, err := buildCachedBoundaryGraphForTest([]string{"."}, 0, true); err != nil || cache != "miss" {
		t.Fatalf("initial cache state=%q err=%v", cache, err)
	}
	if err := os.WriteFile(modulePath, []byte("module example.com/two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, cache, err := buildCachedBoundaryGraphForTest([]string{"."}, 0, true); err != nil || cache != "miss" {
		t.Fatalf("identity cache state=%q err=%v", cache, err)
	}
}

func writeBoundaryFixture(t *testing.T, dir string) {
	t.Helper()
	writeGraphSource(t, dir, "owner.go", "package sample\nfunc Parse(){}\nfunc Validate(){}\n")
	writeGraphSource(t, dir, "one.go", "package sample\nfunc One(){ Parse(); Validate() }\n")
	writeGraphSource(t, dir, "two.go", "package sample\nfunc Two(){ Parse(); Validate() }\n")
}
