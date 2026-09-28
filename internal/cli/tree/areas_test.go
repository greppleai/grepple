package tree

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func TestLocalTreeShowsCurrentAreasOnFilesAndAncestors(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, item := range []struct {
		directory, name, kind string
		areas                 []string
		stale                 bool
	}{
		{root, "main.go", "production", []string{"frontend"}, false},
		{filepath.Join(root, "pkg"), "service.go", "production", []string{"shared", "backend"}, false},
		{filepath.Join(root, "pkg"), "old.go", "production", []string{"retired"}, true},
		{filepath.Join(root, "pkg", "inner"), "spec.go", "test", []string{"tests", "shared"}, false},
	} {
		if err := os.MkdirAll(item.directory, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(item.directory, item.name)
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		checksum, err := filedigest.SHA256Hex(path)
		if err != nil {
			t.Fatal(err)
		}
		if item.stale {
			checksum = "outdated"
		}
		metadata, err := directorymeta.Read(root, item.directory)
		if os.IsNotExist(err) {
			metadata = directorymeta.Metadata{Description: "Source directory.", Responsibilities: []string{"Own source."}}
		} else if err != nil {
			t.Fatal(err)
		}
		metadata.Files = append(metadata.Files, directorymeta.File{Path: item.name, Kind: item.kind, Description: item.name, Checksum: checksum, Areas: item.areas})
		if err := directorymeta.Write(root, item.directory, metadata); err != nil {
			t.Fatal(err)
		}
	}
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, nil)
	for _, test := range []struct {
		depth int
		kind  sourcedomain.Kind
		root  []string
		want  map[string][]string
	}{
		{1, "", []string{"backend", "frontend", "shared", "tests"}, map[string][]string{"main.go": {"frontend"}, "pkg": {"backend", "shared", "tests"}}},
		{2, "", []string{"backend", "frontend", "shared", "tests"}, map[string][]string{"main.go": {"frontend"}, "pkg": {"backend", "shared", "tests"}, "pkg/service.go": {"backend", "shared"}, "pkg/old.go": nil, "pkg/inner": {"shared", "tests"}}},
		{3, "", []string{"backend", "frontend", "shared", "tests"}, map[string][]string{"main.go": {"frontend"}, "pkg": {"backend", "shared", "tests"}, "pkg/service.go": {"backend", "shared"}, "pkg/old.go": nil, "pkg/inner": {"shared", "tests"}, "pkg/inner/spec.go": {"shared", "tests"}}},
		{1, sourcedomain.Test, []string{"shared", "tests"}, map[string][]string{"pkg": {"shared", "tests"}}},
		{1, sourcedomain.Production, []string{"backend", "frontend", "shared"}, map[string][]string{"main.go": {"frontend"}, "pkg": {"backend", "shared"}}},
	} {
		response, err := buildLocal(root, test.depth, test.kind, nil, repository)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(response.Areas, test.root) {
			t.Fatalf("depth=%d kind=%s root areas=%v want=%v", test.depth, test.kind, response.Areas, test.root)
		}
		entries := map[string][]string{}
		for _, entry := range response.Entries {
			if entry.Path == ".grepple" || entry.Path == directorymeta.RepositoryFile {
				t.Fatal("internal metadata leaked into tree")
			}
			entries[entry.Path] = entry.Areas
		}
		if !reflect.DeepEqual(entries, test.want) {
			t.Fatalf("depth=%d kind=%s entries=%v want=%v", test.depth, test.kind, entries, test.want)
		}
	}
	for _, test := range []struct {
		selected []string
		kind     sourcedomain.Kind
		depth    int
		root     []string
		want     map[string][]string
	}{
		{[]string{"backend"}, "", 1, []string{"backend", "shared"}, map[string][]string{"pkg": {"backend", "shared"}}},
		{[]string{"tests"}, "", 1, []string{"shared", "tests"}, map[string][]string{"pkg": {"shared", "tests"}}},
		{[]string{"frontend", "tests"}, "", 1, []string{"frontend", "shared", "tests"}, map[string][]string{"main.go": {"frontend"}, "pkg": {"shared", "tests"}}},
		{[]string{"shared"}, "", 2, []string{"backend", "shared", "tests"}, map[string][]string{"pkg": {"backend", "shared", "tests"}, "pkg/inner": {"shared", "tests"}, "pkg/service.go": {"backend", "shared"}}},
		{[]string{"backend"}, sourcedomain.Test, 1, nil, map[string][]string{}},
		{[]string{"retired"}, "", 1, nil, map[string][]string{}},
	} {
		response, err := buildLocal(root, test.depth, test.kind, test.selected, repository)
		if err != nil {
			t.Fatal(err)
		}
		entries := map[string][]string{}
		for _, entry := range response.Entries {
			entries[entry.Path] = entry.Areas
		}
		if !reflect.DeepEqual(response.Areas, test.root) || !reflect.DeepEqual(entries, test.want) {
			t.Fatalf("--area %v kind=%s depth=%d root=%v entries=%v want root=%v entries=%v", test.selected, test.kind, test.depth, response.Areas, entries, test.root, test.want)
		}
	}
	file, err := buildLocal(filepath.Join(root, "pkg", "service.go"), 1, "", nil, repository)
	if err != nil || !reflect.DeepEqual(file.Areas, []string{"backend", "shared"}) || len(file.Entries) != 1 || !reflect.DeepEqual(file.Entries[0].Areas, file.Areas) {
		t.Fatalf("explicit file tree=%+v err=%v", file, err)
	}
	unmatched, err := buildLocal(filepath.Join(root, "pkg", "service.go"), 1, "", []string{"frontend"}, repository)
	if err != nil || len(unmatched.Entries) != 0 {
		t.Fatalf("unmatched explicit file tree=%+v err=%v", unmatched, err)
	}
}
