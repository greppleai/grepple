package artifacts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestArtifactsCleanRemovesPersistedOutput(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, ".grepple", "output")
	t.Setenv("GREPPLE_ARTIFACT_DIR", output)
	if err := os.MkdirAll(output, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "one.json"), []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "two.txt"), []byte("12"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := cliruntime.Environment{Output: &stdout, RepositoryContext: cliruntime.RepositoryServices{WorkingDirectoryFunc: func() string { return root }}}
	if err := New(application).Run([]string{"clean", "--json"}); err != nil {
		t.Fatal(err)
	}
	var result CleanOutput
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Files != 2 || result.Bytes != 6 {
		t.Fatalf("cleanup = %+v", result)
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 0 {
		t.Fatalf("remaining artifacts=%d err=%v", len(entries), err)
	}
}
