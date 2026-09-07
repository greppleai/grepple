package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchFilesExcludesRepository(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"owner/current/file.go", "owner/other/file.go"} {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte("package sample\n// needle\n"), 0644); err != nil {
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

	params := Params{
		Query:       "needle",
		MaxSegments: DefaultMaxSegments,
		ExcludeRepo: []string{"owner/current"},
	}
	matches, err := Files(params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].DisplayPath != "owner/other/file.go" {
		t.Fatalf("matches=%#v", matches)
	}
	paths, err := ListFilePaths(Params{Files: true, ExcludeRepo: params.ExcludeRepo}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "owner/other/file.go" {
		t.Fatalf("paths=%v", paths)
	}
}
