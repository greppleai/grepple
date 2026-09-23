package sources

import (
	"os"

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
