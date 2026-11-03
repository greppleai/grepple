package overview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestOverviewFastPathDoesNotReplaceParentOrExistingMetadataInventory(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "checkout")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	source(t, root, "main.go", directorymeta.FileV1{})
	application, _ := contextFor(t, parent)
	report, err := Build(application)
	if err != nil || !report.CountsComputed || report.Files.Selected != 1 {
		t.Fatalf("parent must retain full source discovery: %+v err=%v", report, err)
	}
	if err := os.Mkdir(filepath.Join(root, ".grepple"), 0700); err != nil {
		t.Fatal(err)
	}
	application, _ = contextFor(t, root)
	report, err = Build(application)
	if err != nil || !report.CountsComputed || report.Files.Missing != 1 {
		t.Fatalf("existing .grepple without YAML must retain coverage: %+v err=%v", report, err)
	}
}
