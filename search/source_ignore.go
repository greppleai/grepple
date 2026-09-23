package search

import (
	"os"

	"github.com/greppleai/grepple/internal/pathfilter"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type sourceIgnoreConfig struct {
	root           string
	patterns       []string
	productionOnly bool
}

func (config sourceIgnoreConfig) ignored(candidate string) bool {
	if config.builtIn(candidate) {
		return true
	}
	if config.productionOnly && !sourcedomain.IsProduction(candidate, config.root) {
		return true
	}
	filter := config.filter()
	return filter.Ignored(candidate)
}

func (config sourceIgnoreConfig) builtIn(candidate string) bool {
	filter := config.filter()
	relative, inRepository := filter.Relative(candidate)
	if !inRepository {
		return false
	}
	return pathfilter.Match(".grepple/**", relative) || pathfilter.Match(".worktrees/**", relative)
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
