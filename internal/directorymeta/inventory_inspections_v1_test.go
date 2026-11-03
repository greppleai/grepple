package directorymeta

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/filedigest"
)

func inventoryInspectionFixtureV1(t testing.TB, count int) (string, []string, map[string][]string, DocumentV1) {
	t.Helper()
	root := t.TempDir()
	directories := make([]string, 0, count)
	paths := map[string][]string{}
	document := DocumentV1{Schema: "v1", Directory: map[string]DirectoryV1{}}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("dir%03d", i)
		directory := filepath.Join(root, key)
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		directories = append(directories, directory)
		entry := DirectoryV1{Description: key}
		for _, name := range []string{"current.go", "stale.go", "unreadable.go", "unannotated.go"} {
			path := filepath.Join(directory, name)
			if err := os.WriteFile(path, []byte("package sample\n"), 0600); err != nil {
				t.Fatal(err)
			}
			paths[directory] = append(paths[directory], path)
			if name == "unannotated.go" {
				continue
			}
			digest, err := filedigest.SHA256Hex(path)
			if err != nil {
				t.Fatal(err)
			}
			entry.Files = append(entry.Files, FileV1{Name: name, Checksum: digest, Description: name, Purpose: "implementation", DomainAreas: []string{"inventory"}})
		}
		if err := os.WriteFile(filepath.Join(directory, "stale.go"), []byte("changed\n"), 0600); err != nil {
			t.Fatal(err)
		}
		// A vanished selected file is an invalid read, even when running as root.
		if err := os.Remove(filepath.Join(directory, "unreadable.go")); err != nil {
			t.Fatal(err)
		}
		entry.Files = append(entry.Files, FileV1{Name: "removed.go", Checksum: entry.Files[0].Checksum, Description: "Removed file.", Purpose: "implementation"})
		document.Directory[key] = entry
	}
	return root, directories, paths, document
}

func TestInventoryInspectionsMatchSequentialStatusesAndOrder(t *testing.T) {
	root, directories, paths, document := inventoryInspectionFixtureV1(t, 24)
	for _, loadErr := range []error{nil, os.ErrNotExist, fmt.Errorf("invalid metadata")} {
		want := make([]InspectionV1, len(directories))
		for index, directory := range directories {
			want[index] = inspectDirectorySnapshotV1(root, directory, paths[directory], document, loadErr)
		}
		for range 10 {
			got := inspectInventoryDirectoriesV1(root, directories, paths, document, loadErr)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("parallel inspection changed statuses/order (load error %v)", loadErr)
			}
		}
	}
}

func TestBuildInventoryParallelInspectionPreservesResults(t *testing.T) {
	root, _, paths, document := inventoryInspectionFixtureV1(t, 24)
	if err := WriteDocumentV1(root, document); err != nil {
		t.Fatal(err)
	}
	document, err := ReadDocumentV1(root)
	if err != nil {
		t.Fatal(err)
	}
	var selected []string
	for _, files := range paths {
		selected = append(selected, files...)
	}
	directories, err := Directories(root, selected)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]InspectionV1{}
	for _, directory := range directories {
		key, err := DirectoryKey(root, directory)
		if err != nil {
			t.Fatal(err)
		}
		want[key] = inspectDirectorySnapshotV1(root, directory, paths[directory], document, nil)
	}
	var previous InventoryV1
	for iteration := range 5 {
		got, err := BuildInventoryV1(root, selected)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Directories, want) {
			t.Fatal("inventory differs from sequential directory inspection")
		}
		if iteration > 0 && !reflect.DeepEqual(got, previous) {
			t.Fatal("concurrent inspection changed file/label order or statuses")
		}
		if !reflect.DeepEqual(got.Labels["."].DomainAreas, []string{"inventory"}) {
			t.Fatalf("ancestor labels: %+v", got.Labels["."])
		}
		previous = got
	}
	// Every invocation must rehash content, including same-length edits.
	path := filepath.Join(root, "dir000", "current.go")
	if err := os.WriteFile(path, []byte("package edited\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := BuildInventoryV1(root, selected)
	if err != nil || got.Files["dir000/current.go"].Status != StatusStale {
		t.Fatalf("new inventory reused an old checksum: %+v, %v", got.Files["dir000/current.go"], err)
	}
}

func BenchmarkInventoryInspectionsV1(b *testing.B) {
	root, directories, paths, document := inventoryInspectionFixtureV1(b, 128)
	for _, parallel := range []bool{false, true} {
		b.Run(fmt.Sprintf("parallel=%t", parallel), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if parallel {
					inspectInventoryDirectoriesV1(root, directories, paths, document, nil)
				} else {
					for _, directory := range directories {
						inspectDirectorySnapshotV1(root, directory, paths[directory], document, nil)
					}
				}
			}
		})
	}
}
