package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
	chdirForConfigTest(t, root)

	text := captureStdout(t, func() {
		if err := Run([]string{"artifacts", "clean", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var result artifactsCleanOutput
	if err := json.Unmarshal([]byte(text), &result); err != nil {
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
