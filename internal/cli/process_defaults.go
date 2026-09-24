package cli

import (
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/storagepaths"
	"github.com/greppleai/grepple/parser"
)

// configureProcessDefaults applies CLI-only engine defaults before constructing
// an invocation. Explicit environment values, including empty values, win.
func configureProcessDefaults() {
	if _, configured := os.LookupEnv(parser.NavigationCacheDirectoryEnv); !configured {
		cwd, err := os.Getwd()
		if err == nil {
			_ = os.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(storagepaths.Cache(cwd), "navigation"))
		}
	}
}
