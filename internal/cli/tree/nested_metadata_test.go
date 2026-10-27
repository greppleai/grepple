package tree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestTreeFromParentUsesNestedCheckoutMetadata(t *testing.T) {
	parent := t.TempDir()
	t.Chdir(parent)
	root := filepath.Join(parent, "checkout")
	file := treeSelectedSource(t, root, "pkg/item.go", directorymeta.FileV1{Purpose: "implementation", DomainAreas: []string{"navigation"}})
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Description: "Contains the checkout."}); err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "pkg"), directorymeta.DirectoryV1{Description: "Implements navigation.", Files: []directorymeta.FileV1{file}}); err != nil {
		t.Fatal(err)
	}
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, nil)
	selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{DomainAreas: []string{"navigation"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path  string
		depth int
		want  string
	}{
		{parent, 1, "checkout"},
		{root, 1, "pkg"},
	} {
		response, err := buildLocalSelected(tc.path, tc.depth, selector, repository)
		if err != nil || len(response.Entries) != 1 || response.Entries[0].Path != tc.want || response.Entries[0].MetadataStatus != directorymeta.StatusCurrent || response.Entries[0].Labels == nil {
			t.Fatalf("tree %q: %+v err=%v", tc.path, response, err)
		}
		if tc.want == "checkout" && response.Entries[0].Description != "Contains the checkout." || tc.want == "pkg" && (response.Entries[0].Description != "Implements navigation." || response.Description != "Contains the checkout.") {
			t.Fatalf("missing directory description: %+v", response)
		}
	}
	unfiltered, err := buildLocalSelected(parent, 1, directorymeta.SelectorV1{}, repository)
	if err != nil || len(unfiltered.Entries) != 1 || unfiltered.Entries[0].MetadataStatus != directorymeta.StatusCurrent || unfiltered.Entries[0].Description != "Contains the checkout." {
		t.Fatalf("unfiltered parent tree: %+v err=%v", unfiltered, err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "item.go"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	response, err := buildLocalSelected(parent, 1, selector, repository)
	if err != nil || len(response.Entries) != 0 {
		t.Fatalf("stale nested file matched selector: %+v err=%v", response, err)
	}
}
