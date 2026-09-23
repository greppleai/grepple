package cli

import (
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/parser"
)

// configureProcessDefaults applies CLI-only engine defaults before constructing
// an invocation. Explicit environment values, including empty values, win.
func configureProcessDefaults() {
	if _, configured := os.LookupEnv(parser.NavigationCacheDirectoryEnv); !configured {
		_ = os.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(".grepple", "cache", "navigation"))
	}
}
