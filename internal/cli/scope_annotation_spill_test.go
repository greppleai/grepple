package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnnotatedSearchSpillKeepsCompleteDecoratedRows(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("sample.go", []byte("package p\nfunc run() {\n work()\n work()\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"search", "--line-only", "-F", "work", "sample.go"}
	direct := captureStdout(t, func() {
		if err := Run(append(append([]string(nil), args...), "--no-spill")); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Count(direct, " @func:2-5") != 2 || strings.Contains(direct, "source-bytes:") {
		t.Fatalf("direct decorated rows: %q", direct)
	}
	spilled := captureStdout(t, func() {
		if err := Run(append(append([]string(nil), args...), "--spill-threshold-bytes", "64", "--artifact-dir", "artifact-output")); err != nil {
			t.Fatal(err)
		}
	})
	var artifactPath string
	for _, field := range strings.Fields(spilled) {
		if strings.HasPrefix(field, "path=") {
			artifactPath = strings.TrimPrefix(field, "path=")
			break
		}
	}
	if artifactPath == "" || !strings.Contains(spilled, "rerun: grepple search --line-only") || strings.Contains(spilled, "--annotate-scopes") {
		t.Fatalf("lost default on spill replay: %q", spilled)
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifactPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != direct {
		t.Fatalf("artifact mutated framed rows:\n%s\nwant:\n%s", content, direct)
	}
}
