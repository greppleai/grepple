package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

func TestInspectSourceScopeExplainsIgnoresProductionAndExplicitBypass(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "main.go")
	writeScopeFile(t, root, "main_test.go")
	writeScopeFile(t, root, "sandbox/ignored.go")
	writeScopeFile(t, root, ".grepple/cache/facts.bin")
	writeScopeFile(t, root, ".gitignore")
	writeScopeMetadata(t, root, map[string]string{"main.go": "production", "main_test.go": "test"})
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, root, "ignored.txt")

	options := SourceScopeOptions{Root: root, IgnoreRoot: root, IgnorePaths: []string{"sandbox/**"}, ProductionOnly: true}
	decisions, err := InspectSourceScope([]string{root}, options)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]SourcePathDecision{}
	for _, decision := range decisions {
		byPath[decision.Path] = decision
	}
	assertScopeDecision(t, byPath["main.go"], true, "selected")
	assertScopeDecision(t, byPath["main_test.go"], false, "non-production")
	assertScopeDecision(t, byPath["sandbox/ignored.go"], false, "config-ignore")
	assertScopeDecision(t, byPath["ignored.txt"], false, "gitignore")
	if _, included := byPath[".grepple"]; included {
		t.Fatalf("unconditional subtree should not affect path decisions: %+v", byPath[".grepple"])
	}

	explicit, err := InspectSourceScope([]string{filepath.Join(root, "sandbox", "ignored.go")}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit) != 1 || !explicit[0].Selected || explicit[0].Reason != "selected-explicit-config-bypass" {
		t.Fatalf("explicit decision=%+v", explicit)
	}
	explicitTest, err := InspectSourceScope([]string{filepath.Join(root, "main_test.go")}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(explicitTest) != 1 || explicitTest[0].Reason != "selected-explicit-production-bypass" {
		t.Fatalf("explicit test decision=%+v", explicitTest)
	}
	explicitBuiltIn, err := InspectSourceScope([]string{filepath.Join(root, ".grepple", "cache", "facts.bin")}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(explicitBuiltIn) != 1 || explicitBuiltIn[0].Selected || explicitBuiltIn[0].Reason != "built-in-subtree" {
		t.Fatalf("explicit built-in decision=%+v", explicitBuiltIn)
	}
}

func writeScopeFile(t *testing.T, root, relative string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeScopeMetadata(t *testing.T, root string, kinds map[string]string) {
	t.Helper()
	files := make([]directorymeta.File, 0, len(kinds))
	for name, kind := range kinds {
		digest, err := filedigest.SHA256Hex(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, directorymeta.File{Path: name, Description: "Scope fixture.", Kind: kind, Checksum: digest})
	}
	if err := directorymeta.Write(root, root, directorymeta.Metadata{Description: "Scope fixtures.", Responsibilities: []string{"Test source scope."}, Files: files}); err != nil {
		t.Fatal(err)
	}
}
func assertScopeDecision(t *testing.T, decision SourcePathDecision, selected bool, reason string) {
	t.Helper()
	if decision.Selected != selected || decision.Reason != reason {
		t.Fatalf("decision=%+v, want selected=%t reason=%s", decision, selected, reason)
	}
}
