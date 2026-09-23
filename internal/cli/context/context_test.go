package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestRunParsesInvalidationReason(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	t.Setenv("PI_SESSION_ID", "")
	application := cliruntime.Environment{}
	if err := New(application).Run([]string{"invalidate", "--reason", "compact"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "stats-0.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"resetReason": "compact"`) {
		t.Fatalf("stats=%s", content)
	}
	if err := New(application).Run([]string{"invalidate", "--unknown"}); err == nil {
		t.Fatal("expected unknown argument error")
	}
}
