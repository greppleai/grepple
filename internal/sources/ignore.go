package sources

import (
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/pathfilter"
)

type ignoreConfig struct {
	root           string
	patterns       []string
	productionOnly bool
	classifier     *Classifier
}

func (c ignoreConfig) builtIn(candidate string) bool {
	relative, inside := c.filter().Relative(candidate)
	return inside && (pathfilter.Match(".grepple/**", relative) || pathfilter.Match(".worktrees/**", relative))
}

// IsRepositoryMetadataSnapshot recognizes Grepple's generated annotation file,
// even through a symlink alias. Discovery calls this only for candidate files,
// not every file in a recursive walk.
func IsRepositoryMetadataSnapshot(path string) bool {
	isSnapshot := func(name string) bool {
		return filepath.Base(name) == "grepple.yaml" && filepath.Base(filepath.Dir(name)) == ".grepple"
	}
	if isSnapshot(path) {
		return true
	}
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && isSnapshot(resolved)
}
func (c ignoreConfig) filter() pathfilter.Config {
	root := c.root
	if root == "" {
		root, _ = os.Getwd()
	}
	return pathfilter.Config{Root: root, Patterns: c.patterns}
}
func (c ignoreConfig) ignored(candidate string) bool {
	return c.builtIn(candidate) || c.productionOnly && c.kind(candidate) != Production || c.filter().Ignored(candidate)
}

// ignoredDirectory checks only directory-level ignore policy; file kinds cannot
// determine whether a directory contains production files.
func (c ignoreConfig) ignoredDirectory(candidate string) bool {
	return c.builtIn(candidate) || c.filter().Ignored(candidate)
}
func (c ignoreConfig) kind(candidate string) Kind {
	if c.classifier != nil {
		return c.classifier.Classify(candidate)
	}
	return Classify(candidate, c.root)
}
func (c ignoreConfig) hasNegation() bool { return c.filter().HasNegation() }
