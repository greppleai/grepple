package storagepaths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheLivesUnderUserHomeAndRespectsOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GREPPLE_CACHE_DIR", "")
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	first := Cache("/repo/one")
	second := Cache("/repo/two")
	prefix := filepath.Join(userHome, ".grepple", "cache") + string(filepath.Separator)
	if !strings.HasPrefix(first, prefix) || first == second {
		t.Fatalf("repository cache paths: %q %q", first, second)
	}
	if info, err := os.Stat(first); err != nil || !info.IsDir() {
		t.Fatalf("cache directory missing: %v", err)
	}
	custom := filepath.Join(t.TempDir(), "custom")
	t.Setenv("GREPPLE_CACHE_DIR", custom)
	if got := Cache("/repo/one"); got != custom {
		t.Fatalf("override=%q", got)
	}
}
