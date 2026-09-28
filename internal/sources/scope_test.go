package sources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

func TestInspectExplainsIgnoresProductionAndExplicitBypass(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"main.go", "main_test.go", "sandbox/ignored.go", ".grepple/cache/facts.bin", "ignored.txt"} {
		writeScopeFile(t, root, path)
	}
	writeScopeKinds(t, root, map[string]Kind{"main.go": Production, "main_test.go": Test})
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := InspectionOptions{Root: root, IgnoreRoot: root, IgnorePaths: []string{"sandbox/**"}, ProductionOnly: true}
	decisions, err := Inspect([]string{root}, options)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Decision{}
	for _, decision := range decisions {
		byPath[decision.Path] = decision
	}
	assertDecision(t, byPath["main.go"], true, "selected")
	assertDecision(t, byPath["main_test.go"], false, "non-production")
	assertDecision(t, byPath["sandbox/ignored.go"], false, "config-ignore")
	assertDecision(t, byPath["ignored.txt"], false, "gitignore")
	for path, reason := range map[string]string{"sandbox/ignored.go": "selected-explicit-config-bypass", "main_test.go": "selected-explicit-production-bypass"} {
		explicit, err := Inspect([]string{filepath.Join(root, path)}, options)
		if err != nil || len(explicit) != 1 || !explicit[0].Selected || explicit[0].Reason != reason {
			t.Fatalf("explicit %s=%+v err=%v", path, explicit, err)
		}
	}
	explicit, err := Inspect([]string{filepath.Join(root, ".grepple/cache/facts.bin")}, options)
	if err != nil || len(explicit) != 1 || explicit[0].Selected || explicit[0].Reason != "built-in-subtree" {
		t.Fatalf("built-in=%+v err=%v", explicit, err)
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
func writeScopeKinds(t *testing.T, root string, kinds map[string]Kind) {
	t.Helper()
	files := make([]directorymeta.File, 0, len(kinds))
	for name, kind := range kinds {
		digest, err := filedigest.SHA256Hex(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, directorymeta.File{Path: name, Description: "Scope fixture.", Kind: string(kind), Checksum: digest})
	}
	if err := directorymeta.Write(root, root, directorymeta.Metadata{Description: "Scope fixtures.", Responsibilities: []string{"Test source scope."}, Files: files}); err != nil {
		t.Fatal(err)
	}
}

func assertDecision(t *testing.T, decision Decision, selected bool, reason string) {
	t.Helper()
	if decision.Selected != selected || decision.Reason != reason {
		t.Fatalf("decision=%+v, want selected=%t reason=%s", decision, selected, reason)
	}
}
