package tree

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestUninitializedCheckoutTreeDepthAndMissingMetadata(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, directory := range []string{".git", "pkg/deep", "empty", "ignored"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{".gitignore": "ignored/\n*.tmp\n", "main.go": "package main", "pkg/deep/file.go": "package deep", "excluded.tmp": "hidden"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, nil)
	for _, tc := range []struct {
		path  string
		depth int
		want  []string
	}{
		{".", 1, []string{".gitignore", "empty", "main.go", "pkg"}},
		{".", 2, []string{".gitignore", "empty", "main.go", "pkg", "pkg/deep"}},
		{"main.go", 1, []string{"main.go"}},
		{"pkg", 1, []string{"deep"}},
	} {
		response, err := buildLocalSelected(tc.path, tc.depth, directorymeta.SelectorV1{}, repository)
		if err != nil || response.MetadataStatus != directorymeta.StatusMissing {
			t.Fatalf("response=%+v err=%v", response, err)
		}
		var got []string
		for _, entry := range response.Entries {
			got = append(got, entry.Path)
			if entry.MetadataStatus != directorymeta.StatusMissing || entry.Labels != nil {
				t.Fatalf("unexpected metadata: %+v", entry)
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("path=%s depth=%d got=%v want=%v", tc.path, tc.depth, got, tc.want)
		}
	}
	selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{Purposes: []string{"implementation"}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := buildLocalSelected(".", 1, selector, repository)
	if err != nil || len(response.Entries) != 0 || response.MetadataStatus != directorymeta.StatusMissing {
		t.Fatalf("selector must fail closed: %+v err=%v", response, err)
	}
	t.Chdir(filepath.Join(root, "pkg"))
	response, err = buildLocalSelected(".", 1, directorymeta.SelectorV1{}, repository)
	if err != nil || len(response.Entries) != 1 || response.Entries[0].Path != "deep" {
		t.Fatalf("checkout subdirectory: %+v err=%v", response, err)
	}
}
