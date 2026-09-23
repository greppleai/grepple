// Package sources owns repository source policy, discovery, metadata-backed classification, and inspection.
package sources

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/pathfilter"
)

// Options describes resolved repository source policy.
type Options struct {
	WorkingDirectory string
	IgnoreRoot       string
	IgnorePaths      []string
	ProductionOnly   bool
	Notices          io.Writer
}

// Provider supplies repository source policy without depending on a command runtime.
type Provider interface {
	ScopeOptions() (Options, error)
}

// ReportExplicitBypasses reports named files that intentionally bypass broad-source policy.
func ReportExplicitBypasses(paths []string, options Options) {
	root := options.Root()
	filter := pathfilter.Config{Root: root, Patterns: options.IgnorePaths}
	classifier := NewClassifier(root)
	bypasses := make([]string, 0)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		reason := ""
		if filter.Ignored(path) {
			reason = "grepple.json ignore.paths"
		} else if options.ProductionOnly && classifier.Classify(path) != Production {
			reason = "--production-only"
		}
		if reason == "" {
			continue
		}
		display, err := filepath.Rel(options.ResolvedWorkingDirectory(), path)
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

// ResolvedWorkingDirectory returns the invocation working directory or process default.
func (options Options) ResolvedWorkingDirectory() string {
	if options.WorkingDirectory != "" {
		return options.WorkingDirectory
	}
	workingDirectory, _ := os.Getwd()
	return workingDirectory
}

// Root returns the resolved ignore-policy root.
func (options Options) Root() string {
	if options.IgnoreRoot != "" {
		return options.IgnoreRoot
	}
	return options.ResolvedWorkingDirectory()
}
