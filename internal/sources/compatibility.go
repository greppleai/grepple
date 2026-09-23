package sources

// IgnoreCache caches parsed .gitignore files across related path checks.
type IgnoreCache = ignoreCache

// IgnoredFromRoot reports whether path is ignored by repository .gitignore files.
func IgnoredFromRoot(path, root string) bool { return ignoredFromRoot(path, root) }

// IgnoredFromRootCached is IgnoredFromRoot with a caller-owned cache.
func IgnoredFromRootCached(path, root string, cache IgnoreCache) bool {
	return ignoredFromRootCached(path, root, cache)
}

// WithinRoot reports whether path is root or nested beneath it.
func WithinRoot(path, root string) bool { return withinRoot(path, root) }

// ContainsGitDirectory reports whether any path component is .git.
func ContainsGitDirectory(path string) bool { return containsGitDirectory(path) }

// SplitGlobRoots separates directory scopes from file filters.
func SplitGlobRoots(globs []string) ([]string, []string) { return splitGlobRoots(globs) }

// GlobMatch matches a path with support for recursive ** segments.
func GlobMatch(pattern, name string) bool { return globMatch(pattern, name) }
