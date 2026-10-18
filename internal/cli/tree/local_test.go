package tree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestLocalEntriesIncludeFileDescriptions(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{{
		Name: "main.go", Description: "Starts the command-line application.", Purpose: "Implementation", Checksum: "0000000000000000000000000000000000000000000000000000000000000000",
	}}}); err != nil {
		t.Fatal(err)
	}
	inventory, err := directorymeta.BuildInventoryV1(root, []string{file})
	if err != nil {
		t.Fatal(err)
	}
	entries := localEntriesV1([]string{file}, root, root, 1, inventory)
	if len(entries) != 1 || entries[0].Path != "main.go" || entries[0].Description != "Starts the command-line application." || entries[0].MetadataStatus != directorymeta.StatusStale {
		t.Fatalf("entries=%+v", entries)
	}
}
