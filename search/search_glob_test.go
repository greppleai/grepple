package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestCollectCandidateFilesUsesFilepathGlob(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"testdata/x.ts", "testdata/a/x.ts", "other/x.ts"} {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	files, err := collectCandidateFiles([]string{filepath.Join("testdata", "*", "*.ts")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != filepath.Join(root, "testdata", "a", "x.ts") {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestCollectCandidateFilesWithoutGlobWalksRecursively(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "x.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	files, err := collectCandidateFiles(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != path {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestCollectCandidateFilesRecursiveGlob(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"a/x.yaml", "a/b/y.yaml", "a/b/c/z.yml", "a/b/skip.txt"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	previous, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	files, err := collectCandidateFiles([]string{"**/*.yaml"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 yaml files, got %#v", files)
	}
}

func TestCollectCandidateFilesConfinesToRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "repo")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "in.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "secret.go")
	if err := os.WriteFile(outside, []byte("package secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Getwd()
	if err := os.Chdir(inside); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	files, err := collectCandidateFiles([]string{filepath.Join("..", "*.go")}, inside)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected confinement to drop escaping matches, got %#v", files)
	}
}

// writeTree creates the given files (with any parent dirs) under root.
func writeTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func rels(t *testing.T, root string, files []string) []string {
	t.Helper()
	out := make([]string, 0, len(files))
	for _, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out
}

func TestSplitGlobRoots(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "testdata/a.ts", "pkg/main.go")
	chdir(t, root)

	roots, filters := splitGlobRoots([]string{"**/*.ts", "testdata", "pkg/main.go", "missing"})
	if len(roots) != 1 || roots[0] != "testdata" {
		t.Fatalf("roots = %#v, want [testdata]", roots)
	}
	wantFilters := []string{"**/*.ts", "pkg/main.go", "missing"}
	if len(filters) != len(wantFilters) {
		t.Fatalf("filters = %#v, want %#v", filters, wantFilters)
	}
	for i, f := range wantFilters {
		if filters[i] != f {
			t.Fatalf("filters = %#v, want %#v", filters, wantFilters)
		}
	}
}

// TestCollectListingFilesScopesGlobToDirectory guards the fix for
// "grepple -l GLOB DIR": a filtering glob combined with a directory positional must
// scope the glob to that directory instead of unioning every file under it.
func TestCollectListingFilesScopesGlobToDirectory(t *testing.T) {
	root := t.TempDir()
	writeTree(t,
		root,
		"testdata/typescript/a.tsx",
		"testdata/go/main.go",
		"other/b.tsx",
	)
	chdir(t, root)

	files, err := collectListingFiles([]string{"**/*.tsx", "testdata"}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := rels(t, root, files)
	want := []string{"testdata/typescript/a.tsx"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("collectListingFiles scoped glob = %#v, want %#v", got, want)
	}
}

// TestCollectListingFilesDirectoryAloneListsAll keeps the bare-directory listing
// behavior: with no glob filter, every file under the directory is returned.
func TestCollectListingFilesDirectoryAloneListsAll(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "testdata/a.tsx", "testdata/b.go", "other/c.txt")
	chdir(t, root)

	files, err := collectListingFiles([]string{"testdata"}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := rels(t, root, files)
	want := []string{"testdata/a.tsx", "testdata/b.go"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("collectListingFiles dir-only = %#v, want %#v", got, want)
	}
}

// TestCollectListingFilesGlobAloneUnchanged confirms a lone glob still resolves
// exactly like before the scope-root split.
func TestCollectListingFilesGlobAloneUnchanged(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "testdata/a.tsx", "testdata/nested/b.tsx", "testdata/c.go")
	chdir(t, root)

	files, err := collectListingFiles([]string{"**/*.tsx"}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := rels(t, root, files)
	want := []string{"testdata/a.tsx", "testdata/nested/b.tsx"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("collectListingFiles glob-only = %#v, want %#v", got, want)
	}
}

func TestGlobMatchDoubleStar(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"**/*.yaml", "owner/repo/a/b.yaml", true},
		{"**/*.yaml", "owner/repo/a/b.txt", false},
		{"**/*.yaml", "top.yaml", true},
		{"charts/**/values.yaml", "charts/app/nested/values.yaml", true},
		{"charts/**/values.yaml", "other/app/values.yaml", false},
		{"*.go", "main.go", true},
		{"*.go", "pkg/main.go", false},
	}
	for _, test := range cases {
		if got := globMatch(test.pattern, test.name); got != test.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", test.pattern, test.name, got, test.want)
		}
	}
}

func TestListFilePathsContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListFilePathsContext(ctx, Params{}, []string{"main.go"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	root := t.TempDir()
	writeTree(t, root, "main.go")
	chdir(t, root)
	_, err = ListFilePathsContext(ctx, Params{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("discovery error=%v", err)
	}
}
