package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/pathfilter"
	"github.com/greppleai/grepple/internal/sourcekind"
	"github.com/greppleai/grepple/search"
)

func applyRepositorySourceConfig(params *search.Params) error {
	if params == nil {
		return nil
	}
	params.ProductionOnly = activeRepositoryOptions.productionOnly
	params.IgnoreRoot = mustGetwd()
	config, path, err := loadRepositoryConfig()
	if err != nil {
		return err
	}
	if path != "" && !activeRepositoryOptions.ignoreDisabled {
		params.IgnorePaths = append([]string(nil), config.Ignore.Paths...)
		params.IgnoreRoot = filepath.Dir(path)
	}
	explicitPaths := append([]string(nil), params.Globs...)
	if separator := strings.LastIndex(params.At, ":"); separator > 0 {
		explicitPaths = append(explicitPaths, params.At[:separator])
	}
	reportExplicitSourceBypasses(explicitPaths, params.IgnoreRoot, params.IgnorePaths, params.ProductionOnly)
	return nil
}

func configureResolvedSearchParams(params search.Params, err error) (search.Params, error) {
	if err != nil {
		return params, err
	}
	err = applyRepositorySourceConfig(&params)
	return params, err
}

func reportExplicitSourceBypasses(paths []string, root string, patterns []string, productionOnly bool) {
	filter := pathfilter.Config{Root: root, Patterns: patterns}
	bypasses := make([]string, 0)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		reason := ""
		if filter.Ignored(path) {
			reason = "grepple.json ignore.paths"
		} else if productionOnly && !sourcekind.IsProduction(path, root) {
			reason = "--production-only"
		}
		if reason == "" {
			continue
		}
		display, err := filepath.Rel(mustGetwd(), path)
		if err != nil {
			display = path
		}
		bypasses = append(bypasses, reason+"\x00"+filepath.ToSlash(display))
	}
	sort.Strings(bypasses)
	for _, bypass := range bypasses {
		parts := strings.SplitN(bypass, "\x00", 2)
		fmt.Fprintf(os.Stderr, "note: explicitly named file bypasses %s: %s\n", parts[0], parts[1])
	}
}
