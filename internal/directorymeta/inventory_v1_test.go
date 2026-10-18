package directorymeta

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/filedigest"
)

func TestInventoryV1AncestorUnionsExcludeStaleAndUnselectedSources(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a/first.go", "a/deep/current.go", "a/deep/stale.go", "a/deep/unselected.go"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, deep := fixtureDirectoryV1("first.go"), DirectoryV1{}
	first.Files[0].DomainAreas = []string{"Parsing"}
	first.Files[0].FunctionalAreas = []string{"CLI"}
	first.Files[0].Purpose = "Implementation"
	first.Files[0].Keywords = []string{"c++"}
	deep.Files = []FileV1{fixtureDirectoryV1("current.go").Files[0], fixtureDirectoryV1("stale.go").Files[0], fixtureDirectoryV1("unselected.go").Files[0]}
	deep.Files[0].DomainAreas = []string{"parsing", "documentation"}
	deep.Files[0].Purpose = "Testing"
	deep.Files[1].DomainAreas = []string{"StaleOnly"}
	deep.Files[2].DomainAreas = []string{"UnselectedOnly"}
	for _, item := range []struct {
		path  string
		entry *DirectoryV1
	}{{"a/first.go", &first}, {"a/deep/current.go", &deep}} {
		hash, err := filedigest.SHA256Hex(filepath.Join(root, filepath.FromSlash(item.path)))
		if err != nil {
			t.Fatal(err)
		}
		item.entry.Files[0].Checksum = hash
	}
	if err := WriteDirectoryV1(root, filepath.Join(root, "a"), first); err != nil {
		t.Fatal(err)
	}
	if err := WriteDirectoryV1(root, filepath.Join(root, "a/deep"), deep); err != nil {
		t.Fatal(err)
	}
	inventory, err := BuildInventoryV1(root, []string{"a/first.go", "a/deep/current.go", "a/deep/stale.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Files) != 3 || inventory.Files["a/deep/stale.go"].Status != StatusStale {
		t.Fatalf("files: %+v", inventory.Files)
	}
	for _, key := range []string{".", "a"} {
		labels := inventory.Labels[key]
		if len(labels.DomainAreas) != 2 || len(labels.Purposes) != 2 || labels.DomainAreas[0] != "documentation" || labels.DomainAreas[1] != "parsing" || len(labels.Keywords) != 1 {
			t.Fatalf("%q labels: %+v", key, labels)
		}
	}
	deepLabels := inventory.Labels["a/deep"]
	if len(deepLabels.DomainAreas) != 2 || len(deepLabels.FunctionalAreas) != 0 {
		t.Fatalf("deep labels: %+v", deepLabels)
	}
	if _, exists := inventory.Files["a/deep/unselected.go"]; exists {
		t.Fatal("source exclusion leaked into selected inventory")
	}
	if _, exists := inventory.Labels["a/deep/stale.go"]; exists {
		t.Fatal("file treated as directory")
	}
}

func TestInventoryV1DoesNotExcludeUnrelatedGreppleYAML(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "hooks", "bin")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "grepple.yaml")
	if err := os.WriteFile(path, []byte("fixture: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := FilesForDirectoryV1(root, directory, []string{"hooks/bin/grepple.yaml", ".grepple/grepple.yaml"})
	if err != nil || len(files) != 1 || files[0].Name != "grepple.yaml" {
		t.Fatalf("file selection: %+v: %v", files, err)
	}
	if selected := selectedDirectFiles(root, directory, []string{"hooks/bin/grepple.yaml"}); len(selected) != 1 || selected[0] != "grepple.yaml" {
		t.Fatalf("selected files: %v", selected)
	}
}
