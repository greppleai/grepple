package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestConfigureProcessDefaultsPreservesOverridesAndExplicitDisable(t *testing.T) {
	original, configured := os.LookupEnv(parser.NavigationCacheDirectoryEnv)
	t.Cleanup(func() {
		if configured {
			_ = os.Setenv(parser.NavigationCacheDirectoryEnv, original)
		} else {
			_ = os.Unsetenv(parser.NavigationCacheDirectoryEnv)
		}
	})
	t.Setenv("GREPPLE_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))

	if err := os.Unsetenv(parser.NavigationCacheDirectoryEnv); err != nil {
		t.Fatal(err)
	}
	configureProcessDefaults()
	if value := os.Getenv(parser.NavigationCacheDirectoryEnv); value != filepath.Join(os.Getenv("GREPPLE_CACHE_DIR"), "navigation") {
		t.Fatalf("default=%q", value)
	}

	for _, value := range []string{"", "/tmp/custom-navigation-cache"} {
		if err := os.Setenv(parser.NavigationCacheDirectoryEnv, value); err != nil {
			t.Fatal(err)
		}
		configureProcessDefaults()
		if got, present := os.LookupEnv(parser.NavigationCacheDirectoryEnv); !present || got != value {
			t.Fatalf("override=%q present=%v want=%q", got, present, value)
		}
	}
}
