// Package pathfilter applies deterministic repository-relative ignore patterns.
package pathfilter

import (
	"path"
	"path/filepath"
	"strings"
)

// Config is an ordered repository-relative ignore list. Later negated patterns
// re-include earlier matches.
type Config struct {
	Root     string
	Patterns []string
}

// Ignored reports whether candidate is excluded by the configured patterns.
func (config Config) Ignored(candidate string) bool {
	relative, ok := config.Relative(candidate)
	if !ok {
		return false
	}
	ignored := false
	for _, raw := range config.Patterns {
		pattern := strings.TrimSpace(raw)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		negated := strings.HasPrefix(pattern, "!")
		pattern = strings.TrimPrefix(pattern, "!")
		if Match(pattern, relative) {
			ignored = !negated
		}
	}
	return ignored
}

// HasNegation reports whether an ignored directory may contain a re-included descendant.
func (config Config) HasNegation() bool {
	for _, pattern := range config.Patterns {
		if strings.HasPrefix(strings.TrimSpace(pattern), "!") {
			return true
		}
	}
	return false
}

// Relative returns candidate relative to the configured root.
func (config Config) Relative(candidate string) (string, bool) {
	if config.Root == "" {
		return "", false
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return "", false
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(relative)), "./"), true
}

// Match supports slash-separated globs and ** across zero or more path segments.
func Match(raw, candidate string) bool {
	pattern := strings.Trim(strings.TrimSpace(filepath.ToSlash(raw)), "/")
	candidate = strings.Trim(filepath.ToSlash(candidate), "/")
	if pattern == "" {
		return false
	}
	patternParts := strings.Split(pattern, "/")
	candidateParts := []string{}
	if candidate != "" {
		candidateParts = strings.Split(candidate, "/")
	}
	if len(patternParts) == 1 {
		for _, part := range candidateParts {
			if matched, _ := path.Match(pattern, part); matched {
				return true
			}
		}
		return false
	}
	return matchSegments(patternParts, candidateParts)
}

func matchSegments(pattern, candidate []string) bool {
	if len(pattern) == 0 {
		return len(candidate) == 0
	}
	if pattern[0] == "**" {
		return matchSegments(pattern[1:], candidate) || len(candidate) > 0 && matchSegments(pattern, candidate[1:])
	}
	if len(candidate) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], candidate[0])
	return err == nil && matched && matchSegments(pattern[1:], candidate[1:])
}
