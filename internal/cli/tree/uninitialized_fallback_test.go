package tree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestUninitializedTreePreservesParentAndExplicitNestedMetadataDiscovery(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "checkout")
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	file := treeSelectedSource(t, nested, "item.go", directorymeta.FileV1{Purpose: "implementation"})
	if err := os.Mkdir(filepath.Join(nested, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(nested, nested, directorymeta.DirectoryV1{Description: "Nested checkout.", Files: []directorymeta.FileV1{file}}); err != nil {
		t.Fatal(err)
	}
	selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{Purposes: []string{"implementation"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, nil)
	response, err := buildLocalSelected(".", 1, selector, repository)
	if err != nil || len(response.Entries) != 0 || response.MetadataStatus != directorymeta.StatusMissing {
		t.Fatalf("uninitialized root should not discover descendant annotations: %+v err=%v", response, err)
	}
	response, err = buildLocalSelected(nested, 1, selector, repository)
	if err != nil || len(response.Entries) != 1 || response.Entries[0].Path != "item.go" || response.MetadataStatus != directorymeta.StatusCurrent {
		t.Fatalf("explicit annotated nested checkout lost metadata: %+v err=%v", response, err)
	}
	t.Chdir(parent)
	response, err = buildLocalSelected(root, 1, selector, repository)
	if err != nil || len(response.Entries) != 1 || response.Entries[0].Path != "nested" || response.Entries[0].MetadataStatus != directorymeta.StatusCurrent {
		t.Fatalf("parent lost recursive nested discovery: %+v err=%v", response, err)
	}
}
