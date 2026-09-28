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
	if err := directorymeta.Write(root, root, directorymeta.Metadata{
		Description:      "Application entry point.",
		Responsibilities: []string{"Start the application."},
		Files: []directorymeta.File{{
			Path:        "main.go",
			Description: "Starts the command-line application.",
			Checksum:    "checksum",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	entries := localEntries([]string{file}, []string{file}, root, root, 1, nil)
	if len(entries) != 1 || entries[0].Path != "main.go" || entries[0].Description != "Starts the command-line application." || entries[0].MetadataStatus != directorymeta.StatusStale {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestLocalEntryMetadataTreatsLegacyMetadataFileAsOrdinarySource(t *testing.T) {
	root := t.TempDir()
	description, status, _ := localEntryMetadata(root, root, directorymeta.FileName, false, nil, map[string]directorymeta.Inspection{})
	if description != "" || status != directorymeta.StatusMissing {
		t.Fatalf("description=%q status=%q", description, status)
	}
}
