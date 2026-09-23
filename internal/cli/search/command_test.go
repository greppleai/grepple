package search

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestCommandOwnsSearchParsingAndHelp(t *testing.T) {
	var output bytes.Buffer
	command := New(cliruntime.Environment{Output: &output})
	if err := command.Run([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Usage: grepple") || !strings.Contains(output.String(), "--at PATH:LINE[-END]") {
		t.Fatalf("search help=%q", output.String())
	}
}

func TestCommandExecutesItsLocalSearchWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(path, []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := New(cliruntime.Environment{Output: &output})
	if err := command.Run([]string{"--line-only", "-F", "needle", path}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "needle") || !strings.Contains(output.String(), "sample.txt") {
		t.Fatalf("search output=%q", output.String())
	}
}
