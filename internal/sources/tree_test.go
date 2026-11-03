package sources

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnannotatedTreeReadsOnlyRequestedDepthAndHonorsIgnores(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"main.go", "ignored.go", "ignored-dir/item.go", "pkg/child.go", "pkg/deep/leaf.go", "pkg/local.tmp", "pkg/global.tmp", "config-hidden/item.go", ".git/objects/item", ".worktrees/hidden.go"} {
		writeScopeFile(t, root, file)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	for file, content := range map[string]string{".gitignore": "ignored.go\nignored-dir/\nglobal.tmp\n", "pkg/.gitignore": "local.tmp\n"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	policy := Options{WorkingDirectory: root, IgnoreRoot: root, IgnorePaths: []string{"config-hidden/**"}}
	for _, tc := range []struct {
		scope string
		depth int
		reads []string
		want  []string
	}{
		{".", 1, []string{"."}, []string{".gitignore", "empty/", "main.go", "pkg/"}},
		{".", 2, []string{".", "empty", "pkg"}, []string{".gitignore", "empty/", "main.go", "pkg/", "pkg/.gitignore", "pkg/child.go", "pkg/deep/"}},
		{"pkg", 1, []string{"pkg"}, []string{"pkg/.gitignore", "pkg/child.go", "pkg/deep/"}},
	} {
		var reads []string
		read := func(directory string) ([]os.DirEntry, error) {
			rel, _ := filepath.Rel(root, directory)
			reads = append(reads, filepath.ToSlash(rel))
			for _, allowed := range tc.reads {
				if filepath.ToSlash(rel) == allowed {
					return os.ReadDir(directory)
				}
			}
			t.Fatalf("depth %d: read forbidden subtree %s", tc.depth, rel)
			return nil, nil
		}
		paths, err := listTreeWithPolicy(nil, filepath.Join(root, tc.scope), root, tc.depth, policy, read)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, path := range paths {
			rel, _ := filepath.Rel(root, path.Path)
			name := filepath.ToSlash(rel)
			if path.Dir {
				name += "/"
			}
			got = append(got, name)
		}
		if !reflect.DeepEqual(got, tc.want) || !reflect.DeepEqual(reads, tc.reads) {
			t.Fatalf("scope=%s depth=%d: entries=%v reads=%v; want %v %v", tc.scope, tc.depth, got, reads, tc.want, tc.reads)
		}
	}
	// A scoped ignored directory must also honor checkout-ancestor rules.
	paths, err := ListTreeWithPolicy(nil, filepath.Join(root, "ignored-dir"), root, 2, policy)
	if err != nil || len(paths) != 0 {
		t.Fatalf("ignored scope=%v err=%v", paths, err)
	}
}

func TestUnannotatedTreeProductionExplicitFilesAndCancellation(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "file.go")
	policy := Options{WorkingDirectory: root, IgnoreRoot: root, ProductionOnly: true}
	read := func(string) ([]os.DirEntry, error) {
		t.Fatal("production-only must not enumerate unannotated sources")
		return nil, nil
	}
	paths, err := listTreeWithPolicy(nil, root, root, 1, policy, read)
	if err != nil || len(paths) != 0 {
		t.Fatalf("production tree=%v err=%v", paths, err)
	}
	paths, err = listTreeWithPolicy(nil, filepath.Join(root, "file.go"), root, 1, policy, read)
	if err != nil || len(paths) != 1 || paths[0].Dir {
		t.Fatalf("explicit bypass=%v err=%v", paths, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListTreeWithPolicy(ctx, root, root, 1, policy); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}
