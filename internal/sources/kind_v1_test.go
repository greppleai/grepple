package sources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestClassifierV1RejectsOldSnapshotAndUnknownPurposes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "service.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(directorymeta.RepositoryPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directorymeta.RepositoryPath(root), []byte("version: 1\ndirectories:\n  .:\n    files:\n    - path: service.go\n      kind: production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Classify(path, root); got != Unknown {
		t.Fatalf("accepted old schema kind %q", got)
	}
	if err := os.Remove(directorymeta.RepositoryPath(root)); err != nil {
		t.Fatal(err)
	}
	writeKinds(t, root, root, map[string]Kind{"service.go": Production})
	if got := Classify(path, root); got != Production {
		t.Fatalf("ignored current V1 purpose: %q", got)
	}
	if got := Classify(filepath.Join(t.TempDir(), "service.go"), root); got != Unknown {
		t.Fatalf("classified outside root: %q", got)
	}
}
