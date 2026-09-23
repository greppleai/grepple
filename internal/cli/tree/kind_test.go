package tree

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func treeKindFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	pkg := filepath.Join(root, "pkg")
	if err := os.Mkdir(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []directorymeta.File{}
	for _, item := range []struct {
		name, kind string
	}{
		{"spec.go", "test"}, {"prod.go", "production"}, {"fixture.txt", "fixture"}, {"stale.go", "test"}, {"invalid.go", "application"},
	} {
		path := filepath.Join(pkg, item.name)
		if err := os.WriteFile(path, []byte(item.name), 0o600); err != nil {
			t.Fatal(err)
		}
		digest, err := filedigest.SHA256Hex(path)
		if err != nil {
			t.Fatal(err)
		}
		if item.name == "stale.go" {
			digest = "outdated"
		}
		files = append(files, directorymeta.File{Path: item.name, Description: item.name + " description.", Kind: item.kind, Checksum: digest})
	}
	if err := directorymeta.Write(pkg, directorymeta.Metadata{Description: "Source package.", Responsibilities: []string{"Implement behavior."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unclassified.txt"), []byte("unclassified"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLocalTreeFiltersKindsBeforeDepthAndPreservesMetadata(t *testing.T) {
	root := treeKindFixture(t)
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, nil)
	response, err := buildLocal(root, 2, sourcedomain.Test, repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Entries) != 2 || response.Entries[0].Path != "pkg" || !response.Entries[0].Dir || response.Entries[1].Path != "pkg/spec.go" || response.Entries[1].MetadataStatus != directorymeta.StatusCurrent {
		t.Fatalf("test-only entries=%+v", response.Entries)
	}
	depthOne, err := buildLocal(root, 1, sourcedomain.Test, repository)
	if err != nil || len(depthOne.Entries) != 1 || depthOne.Entries[0].Path != "pkg" {
		t.Fatalf("depth-one entries=%+v err=%v", depthOne.Entries, err)
	}
	unknown, err := buildLocal(root, 2, sourcedomain.Unknown, repository)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, entry := range unknown.Entries {
		paths[entry.Path] = true
	}
	if !paths["unclassified.txt"] || !paths["pkg/stale.go"] || !paths["pkg/invalid.go"] || paths["pkg/spec.go"] || paths["pkg/prod.go"] {
		t.Fatalf("unknown entries=%+v", unknown.Entries)
	}
	file, err := buildLocal(filepath.Join(root, "pkg", "spec.go"), 1, sourcedomain.Test, repository)
	if err != nil || len(file.Entries) != 1 || file.Entries[0].Path != "spec.go" {
		t.Fatalf("explicit file entries=%+v err=%v", file.Entries, err)
	}
	noMatch, err := buildLocal(filepath.Join(root, "pkg"), 2, sourcedomain.Vendor, repository)
	if err != nil || len(noMatch.Entries) != 0 {
		t.Fatalf("unmatched entries=%+v err=%v", noMatch.Entries, err)
	}
}

func TestTreeKindArgumentValidationAndRemoteRejection(t *testing.T) {
	var output bytes.Buffer
	calledKind := sourcedomain.Kind("")
	application := cliruntime.Environment{Output: &output}
	cmd := &command{context: application, local: func(path string, depth int, kind sourcedomain.Kind) (api.TreeResponse, error) {
		calledKind = kind
		return api.TreeResponse{Repo: ".", Entries: []api.TreeEntry{{Path: "spec.go"}}}, nil
	}}
	if err := cmd.Run([]string{"--kind", "TEST", "pkg"}); err != nil || calledKind != sourcedomain.Test {
		t.Fatalf("parsed kind=%q err=%v", calledKind, err)
	}
	for _, arguments := range [][]string{
		{"--kind", "application", "pkg"},
		{"--kind", "test", "--repo", "owner/repo", "pkg"},
		{"--kind", "test", "owner/repo", "pkg"},
	} {
		if err := cmd.Run(arguments); err == nil || !strings.Contains(err.Error(), "--kind") {
			t.Fatalf("arguments %q: expected kind error, got %v", arguments, err)
		}
	}
}

func TestTreeKindRejectsProductionOnlyConflict(t *testing.T) {
	root := treeKindFixture(t)
	repository := cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{ProductionOnly: true}, nil)
	if _, err := buildLocal(root, 2, sourcedomain.Test, repository); err == nil || !strings.Contains(err.Error(), "--production-only") {
		t.Fatalf("conflict error=%v", err)
	}
}
