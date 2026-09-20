package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDispatchesWriteCommand(t *testing.T) {
	root := t.TempDir()
	request := `{"schema":"grepple-write-v1","files":[{"path":"created.txt","operation":"create","content_lines":["created"]}]}`
	withStdin(t, request, func() {
		output := captureStdout(t, func() {
			if err := Run([]string{"write", "--root", root}); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(output, "applied 1 files") {
			t.Fatalf("output=%q", output)
		}
	})
	content, err := os.ReadFile(filepath.Join(root, "created.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "created" {
		t.Fatalf("content=%q", content)
	}
}
