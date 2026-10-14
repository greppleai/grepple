package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPagedNavigationSnapshotRelatedParity(t *testing.T) {
	root := t.TempDir()
	paths := []string{}
	for name, content := range map[string]string{"root.go": "package sample\ntype Widget struct{}\nfunc Root(w Widget){Middle(w)}\n", "middle.go": "package sample\nfunc Middle(w Widget){Leaf(w)}\nfunc Leaf(w Widget){}\nfunc Caller(){Root(Widget{})}\n", "extra.py": "def leaf():\n    pass\n"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	artifact, stats := BuildNavigationArtifact(paths)
	if stats.Failed != 0 {
		t.Fatal(stats)
	}
	parts, err := PartitionNavigationArtifact(artifact)
	if err != nil || len(parts) != 2 {
		t.Fatalf("language partitions: %d %v", len(parts), err)
	}
	store := NewNavigationSnapshotStore()
	for _, part := range parts {
		if part.Languages[0] != "go" {
			continue
		}
		path := filepath.Join(t.TempDir(), "paged.gnav")
		key := strings.Repeat("c", 64)
		if err := store.SavePaged(path, key, part.Artifact, nil); err != nil {
			t.Fatal(err)
		}
		snapshot, _, err := store.OpenPaged(path, key)
		if err != nil {
			t.Fatal(err)
		}
		defer snapshot.Close()
		for _, depth := range []int{0, 1, 2, 3} {
			assertSnapshotSearchParity(t, snapshot, root, depth)
		}
	}
}
