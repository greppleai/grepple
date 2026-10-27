package initcommand

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestInitPlansDescriptionForLegacyDirectory(t *testing.T) {
	root, application, _ := initTestContext(t)
	writeInitSource(t, root, "pkg/one.go", "package pkg\n")
	path := filepath.Join(root, "pkg")
	files, err := directorymeta.FilesForDirectoryV1(root, path, []string{"pkg/one.go"})
	if err != nil {
		t.Fatal(err)
	}
	files[0].Description, files[0].Purpose = "Implements the package.", "implementation"
	if err := directorymeta.WriteDirectoryV1(root, path, directorymeta.DirectoryV1{Files: files}); err != nil {
		t.Fatal(err)
	}
	plan, err := planGenerationV1(context.Background(), application, nil, false, "pkg")
	if err != nil || len(plan) != 1 || plan[0].skip || !plan.needsGeneration() {
		t.Fatalf("legacy directory should be refreshed: %+v err=%v", plan, err)
	}
	if err := directorymeta.WriteDirectoryV1(root, path, directorymeta.DirectoryV1{Description: "Implements the package.", Files: files}); err != nil {
		t.Fatal(err)
	}
	plan, err = planGenerationV1(context.Background(), application, nil, false, "pkg")
	if err != nil || len(plan) != 1 || !plan[0].skip {
		t.Fatalf("described directory should be skipped: %+v err=%v", plan, err)
	}
}

func TestInitDescribesDirectoryFromSelectedDescendants(t *testing.T) {
	root, application, _ := initTestContext(t)
	writeInitSource(t, root, "pkg/one.go", "package pkg\n")
	plan, err := planGenerationV1(context.Background(), application, nil, false, "")
	if err != nil || len(plan) != 2 || len(plan[0].files) != 0 || len(plan[0].descendants) != 1 || !plan.needsGeneration() {
		t.Fatalf("ancestor should describe selected descendant: %+v err=%v", plan, err)
	}
	prompt, err := generationPromptV1(root, root, plan[0].files, plan[0].descendants...)
	if err != nil || !strings.Contains(prompt, `"pkg/one.go"`) || !strings.Contains(prompt, "return files: []") {
		t.Fatalf("descendant prompt=%q err=%v", prompt, err)
	}
	entry, err := parseGeneratedV1("description: Contains the pkg implementation.\nfiles: []", nil, directorymeta.DirectoryV1{})
	if err != nil || entry.Description == "" || len(entry.Files) != 0 {
		t.Fatalf("ancestor generation=%+v err=%v", entry, err)
	}
}
