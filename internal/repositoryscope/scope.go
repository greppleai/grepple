// Package repositoryscope applies repository source-selection policy consistently.
package repositoryscope

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	codeextract "github.com/greppleai/grepple/extract"
	"github.com/greppleai/grepple/internal/pathfilter"
	"github.com/greppleai/grepple/internal/sourcekind"
	"github.com/greppleai/grepple/search"
)

// Options describes resolved repository source policy.
type Options struct {
	WorkingDirectory string
	IgnoreRoot       string
	IgnorePaths      []string
	ProductionOnly   bool
	Notices          io.Writer
}

// Configure applies source policy to search parameters and reports explicit bypasses.
func Configure(params *search.Params, options Options) {
	if params == nil {
		return
	}
	params.ProductionOnly = options.ProductionOnly
	params.IgnoreRoot = options.root()
	params.IgnorePaths = append([]string(nil), options.IgnorePaths...)
	explicit := append([]string(nil), params.Globs...)
	if separator := strings.LastIndex(params.At, ":"); separator > 0 {
		explicit = append(explicit, params.At[:separator])
	}
	ReportExplicitBypasses(explicit, options)
}

// LoadSources loads extraction inputs under the same source policy used by search.
func LoadSources(roots []string, options Options) ([]codeextract.Source, error) {
	ReportExplicitBypasses(roots, options)
	return codeextract.LoadSourcesWithOptions(roots, codeextract.DiscoveryOptions{IgnoreRoot: options.root(), IgnorePaths: append([]string(nil), options.IgnorePaths...), ProductionOnly: options.ProductionOnly})
}

// ReportExplicitBypasses reports named files that intentionally bypass broad-source policy.
func ReportExplicitBypasses(paths []string, options Options) {
	root := options.root()
	filter := pathfilter.Config{Root: root, Patterns: options.IgnorePaths}
	bypasses := make([]string, 0)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		reason := ""
		if filter.Ignored(path) {
			reason = "grepple.json ignore.paths"
		} else if options.ProductionOnly && !sourcekind.IsProduction(path, root) {
			reason = "--production-only"
		}
		if reason == "" {
			continue
		}
		display, err := filepath.Rel(options.workingDirectory(), path)
		if err != nil {
			display = path
		}
		bypasses = append(bypasses, reason+"\x00"+filepath.ToSlash(display))
	}
	sort.Strings(bypasses)
	for _, bypass := range bypasses {
		parts := strings.SplitN(bypass, "\x00", 2)
		if options.Notices != nil {
			fmt.Fprintf(options.Notices, "note: explicitly named file bypasses %s: %s\n", parts[0], parts[1])
		}
	}
}

func (options Options) workingDirectory() string {
	if options.WorkingDirectory != "" {
		return options.WorkingDirectory
	}
	workingDirectory, _ := os.Getwd()
	return workingDirectory
}

func (options Options) root() string {
	if options.IgnoreRoot != "" {
		return options.IgnoreRoot
	}
	return options.workingDirectory()
}
