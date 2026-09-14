package search

import (
	"os"

	"github.com/greppleai/grepple/internal/pathfilter"
)

type sourceIgnoreConfig struct {
	root     string
	patterns []string
}

func (config sourceIgnoreConfig) ignored(candidate string) bool {
	filter := config.filter()
	relative, inRepository := filter.Relative(candidate)
	if !inRepository {
		return false
	}
	if pathfilter.Match(".grepple/**", relative) {
		return true
	}
	return filter.Ignored(candidate)
}

func (config sourceIgnoreConfig) hasNegation() bool {
	return config.filter().HasNegation()
}

func (config sourceIgnoreConfig) filter() pathfilter.Config {
	root := config.root
	if root == "" {
		root, _ = os.Getwd()
	}
	return pathfilter.Config{Root: root, Patterns: config.patterns}
}
