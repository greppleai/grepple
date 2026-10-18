package directorymeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/filedigest"
)

func TestTreeSummaryV1ReportsMissingSnapshot(t *testing.T) {
	if got := TreeSummary(t.TempDir(), nil, 1); got != ". [metadata: missing]" {
		t.Fatalf("missing metadata summary=%q", got)
	}
}

func TestTreeSummaryOnlyUsesSelectedChecksumCurrentPurposes(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "pkg")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []FileV1{}
	for _, item := range []struct{ name, purpose string }{
		{"selected.go", "Implementation"}, {"stale.go", "StalePurpose"}, {"excluded.go", "ExcludedPurpose"},
	} {
		path := filepath.Join(directory, item.name)
		if err := os.WriteFile(path, []byte(item.name), 0o600); err != nil {
			t.Fatal(err)
		}
		digest, err := filedigest.SHA256Hex(path)
		if err != nil {
			t.Fatal(err)
		}
		if item.name == "stale.go" {
			digest = strings.Repeat("0", 64)
		}
		files = append(files, FileV1{Name: item.name, Description: "Source file.", Purpose: item.purpose, Checksum: digest})
	}
	if err := WriteDirectoryV1(root, directory, DirectoryV1{Files: files}); err != nil {
		t.Fatal(err)
	}
	selected := []string{"pkg/selected.go", "pkg/stale.go"}
	summary := TreeSummary(root, selected, 1)
	if !strings.Contains(summary, ". — purposes: implementation") || !strings.Contains(summary, "pkg/ — purposes: implementation [metadata: stale]") || strings.Contains(summary, "stalepurpose") || strings.Contains(summary, "excludedpurpose") {
		t.Fatalf("untrusted annotations in summary: %q", summary)
	}
	if got := TreeSummary(root, selected, 0); got != "" {
		t.Fatalf("zero depth=%q", got)
	}
	if got := TreeSummary(root, selected, 2); !strings.Contains(got, "pkg/ — purposes: implementation") {
		t.Fatalf("depth two=%q", got)
	}
}
