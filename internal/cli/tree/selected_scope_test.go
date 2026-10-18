package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestTreeSelectorsNeverTrustMissingInvalidOrStaleRepositoryMetadata(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	file := treeSelectedSource(t, root, "pkg/item.go", directorymeta.FileV1{Purpose: "Implementation", FunctionalAreas: []string{"Review"}})
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, nil)
	selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{FunctionalAreas: []string{"review"}})
	if err != nil {
		t.Fatal(err)
	}
	check := func(status string, want int) {
		t.Helper()
		response, err := buildLocalSelected(root, 1, selector, repository)
		if err != nil || len(response.Entries) != want || response.MetadataStatus != status {
			t.Fatalf("status=%s entries=%+v err=%v", status, response, err)
		}
	}
	check(directorymeta.StatusMissing, 0)
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "pkg"), directorymeta.DirectoryV1{Files: []directorymeta.FileV1{file}}); err != nil {
		t.Fatal(err)
	}
	check(directorymeta.StatusMissing, 1)
	path := filepath.Join(root, "pkg", "item.go")
	direct, err := buildLocalSelected(path, 1, selector, repository)
	if err != nil || len(direct.Entries) != 1 || direct.Entries[0].Path != "item.go" || direct.Entries[0].Labels == nil {
		t.Fatalf("explicit current file=%+v err=%v", direct, err)
	}
	other, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{Purposes: []string{"Testing"}})
	if err != nil {
		t.Fatal(err)
	}
	direct, err = buildLocalSelected(path, 1, other, repository)
	if err != nil || len(direct.Entries) != 0 {
		t.Fatalf("explicit unmatched file=%+v err=%v", direct, err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	check(directorymeta.StatusMissing, 0)
	metadata := filepath.Join(root, filepath.FromSlash(directorymeta.RepositoryFile))
	if err := os.WriteFile(metadata, []byte("schema: v1\ndirectory: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	check(directorymeta.StatusInvalid, 0)
	if err := os.Remove(metadata); err != nil {
		t.Fatal(err)
	}
	check(directorymeta.StatusMissing, 0)
}

func TestTreeProductionOnlyAndGitignoreConstrainUniverseBeforeSelectors(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	prod := treeSelectedSource(t, root, "prod.go", directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"Core"}})
	spec := treeSelectedSource(t, root, "spec.go", directorymeta.FileV1{Purpose: "Testing", DomainAreas: []string{"Core"}})
	ignored := treeSelectedSource(t, root, "ignored.go", directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"Ignored"}})
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{prod, spec, ignored}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{ProductionOnly: true}, nil)
	for _, test := range []struct {
		area    string
		count   int
		purpose string
	}{
		{"Core", 1, "Implementation"}, {"Ignored", 0, ""},
	} {
		selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{DomainAreas: []string{test.area}})
		if err != nil {
			t.Fatal(err)
		}
		response, err := buildLocalSelected(root, 1, selector, repository)
		if err != nil || len(response.Entries) != test.count {
			t.Fatalf("area %s entries=%+v err=%v", test.area, response, err)
		}
		if test.count > 0 && (response.Labels == nil || !strings.EqualFold(response.Labels.Purposes[0], test.purpose)) {
			t.Fatalf("area %s labels=%+v", test.area, response.Labels)
		}
	}
	selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{Purposes: []string{"Testing"}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := buildLocalSelected(root, 1, selector, repository)
	if err != nil || len(response.Entries) != 0 {
		t.Fatalf("production-only leaked test %+v err=%v", response, err)
	}
	// Explicitly named ignored sources must remain outside the selected universe.
	selector, err = directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{DomainAreas: []string{"Ignored"}})
	if err != nil {
		t.Fatal(err)
	}
	response, err = buildLocalSelected(filepath.Join(root, "ignored.go"), 1, selector, repository)
	if err != nil || len(response.Entries) != 0 {
		t.Fatalf("explicit ignored file leaked %+v err=%v", response, err)
	}
}
