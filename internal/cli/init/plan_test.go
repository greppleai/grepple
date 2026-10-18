package initcommand

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func initTestContext(t *testing.T) (string, cliruntime.Context, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	var output bytes.Buffer
	return root, cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}}), &output
}

func writeInitSource(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInitRejectsZeroConcurrencyBeforeModelSelection(t *testing.T) {
	_, application, _ := initTestContext(t)
	if err := Execute(application, &Args{Concurrency: 0}); err == nil || !strings.Contains(err.Error(), "--concurrency") {
		t.Fatalf("invalid concurrency error=%v", err)
	}
}
