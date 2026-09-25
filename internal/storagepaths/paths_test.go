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

func TestDaemonCacheIsPrivateAndSharedAcrossRepositories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GREPPLE_CACHE_DIR", "")
	shared := DaemonCache()
	if shared != filepath.Join(home, ".grepple", "cache") {
		t.Fatalf("shared cache=%q", shared)
	}
	if info, err := os.Stat(shared); err != nil || !info.IsDir() {
		t.Fatalf("shared cache directory: %v", err)
	}
	if Cache("/one") == Cache("/two") {
		t.Fatal("repository navigation caches collided")
	}
	t.Setenv("GREPPLE_CACHE_DIR", filepath.Join(t.TempDir(), "custom"))
	if DaemonCache() != os.Getenv("GREPPLE_CACHE_DIR") {
		t.Fatal("absolute cache override ignored")
	}
	t.Setenv("GREPPLE_CACHE_DIR", "relative/cache")
	if DaemonCache() != "" {
		t.Fatal("relative cache override was accepted for user-level daemon")
	}
}
