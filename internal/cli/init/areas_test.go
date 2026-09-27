package initcommand

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestInitInventoryIncludesOtherDirectoriesForOnlyDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, name := range []string{"a/a.go", "b/b.go"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package demo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := directorymeta.FilesForDirectory(root, filepath.Join(root, "b"), []string{"b/b.go"})
	if err != nil {
		t.Fatal(err)
	}
	files[0].Description, files[0].Kind, files[0].Areas = "Owns flow.", "production", []string{"outline"}
	if err := directorymeta.Write(filepath.Join(root, "b"), directorymeta.Metadata{Description: "B.", Responsibilities: []string{"Own flow."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}})
	plan, err := planGeneration(context.Background(), application, nil, false, "a")
	if err != nil || len(plan) != 1 || filepath.Base(plan[0].directory) != "a" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	inventory, err := areaInventory(context.Background(), application, root)
	if err != nil || len(inventory) != 1 || inventory[0].Path != "b/b.go" {
		t.Fatalf("inventory=%+v err=%v", inventory, err)
	}
	prompt, err := generationPromptWithAreas(root, plan[0].directory, plan[0].files, inventory)
	if err != nil || !strings.Contains(prompt, "outline b/b.go") || !strings.Contains(prompt, "Authoritative file manifest") {
		t.Fatalf("prompt=%q err=%v", prompt, err)
	}
}

func TestInitKeepsHandAuthoredTagsAndRequiresEvidenceForAdditions(t *testing.T) {
	files := []directorymeta.File{{Path: "file.go", Checksum: "current"}}
	existing := directorymeta.Metadata{Files: []directorymeta.File{{Path: "file.go", Areas: []string{"outline"}}}}
	base := "description: Code.\nresponsibilities: [Own code.]\nfiles:\n- path: file.go\n  description: Source.\n  kind: production\n  areas: [new-feature]\n"
	if _, err := parseGeneratedMetadataWithExisting(base, files, existing); err == nil {
		t.Fatal("added new area without evidence")
	}
	withoutCitation := base + "area_proposals:\n- path: file.go\n  area: new-feature\n  action: add\n  evidence: 'possibly the source file'\n"
	if _, err := parseGeneratedMetadataWithExisting(withoutCitation, files, existing); err == nil {
		t.Fatal("accepted proposal without local SOURCE:LINE citation")
	}
	withProposal := base + "area_proposals:\n- path: file.go\n  area: new-feature\n  action: add\n  evidence: 'file.go:4-8 contains the entrypoint'\n"
	if _, err := parseGeneratedMetadataWithExisting(strings.Replace(withProposal, "file.go:4-8", "SOURCE:4-8", 1), files, existing); err == nil {
		t.Fatal("accepted placeholder instead of a local filename:line citation")
	}
	metadata, err := parseGeneratedMetadataWithExisting(withProposal, files, existing)
	if err != nil || len(metadata.Files) != 1 || strings.Join(metadata.Files[0].Areas, ",") != "outline,new-feature" || metadata.Files[0].Checksum != "current" {
		t.Fatalf("metadata=%+v err=%v", metadata, err)
	}
	root := t.TempDir()
	if err := directorymeta.Write(root, metadata); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(filepath.Join(root, directorymeta.FileName))
	if err != nil || strings.Contains(string(persisted), "area_proposals") {
		t.Fatalf("persisted=%s err=%v", persisted, err)
	}
	remove := "description: Code.\nresponsibilities: [Own code.]\nfiles:\n- path: file.go\n  description: Source.\n  kind: production\narea_proposals:\n- path: file.go\n  area: outline\n  action: remove\n  evidence: 'file.go:1 is now only a wrapper'\n"
	metadata, err = parseGeneratedMetadataWithExisting(remove, files, existing)
	if err != nil || len(metadata.Files[0].Areas) != 1 || metadata.Files[0].Areas[0] != "outline" || len(metadata.AreaProposals) != 1 {
		t.Fatalf("removal proposal stripped tag: %+v err=%v", metadata, err)
	}
}
