package search

import (
	"context"

	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

// CollectFilesUnder is retained for Search API compatibility.
func CollectFilesUnder(dirs []string, root string) ([]string, error) {
	return sourcedomain.CollectFilesUnder(dirs, root)
}

func collectCandidateFiles(globs []string, root string) ([]string, error) {
	return sourcedomain.Candidates(context.Background(), globs, sourcedomain.DiscoveryOptions{Root: root})
}

func collectCandidateFilesContext(ctx context.Context, globs []string, root string) ([]string, error) {
	return sourcedomain.Candidates(ctx, globs, sourcedomain.DiscoveryOptions{Root: root})
}

func collectCandidateFilesConfiguredContext(ctx context.Context, globs []string, root string, ignore sourceIgnoreConfig) ([]string, error) {
	return sourcedomain.Candidates(ctx, globs, sourcedomain.DiscoveryOptions{Root: root, IgnoreRoot: ignore.root, IgnorePaths: ignore.patterns, ProductionOnly: ignore.productionOnly})
}

func collectListingFiles(globs []string, root string) ([]string, error) {
	return sourcedomain.Listing(context.Background(), globs, sourcedomain.DiscoveryOptions{Root: root})
}

func collectListingFilesContext(ctx context.Context, globs []string, root string) ([]string, error) {
	return sourcedomain.Listing(ctx, globs, sourcedomain.DiscoveryOptions{Root: root})
}

func collectListingFilesConfiguredContext(ctx context.Context, globs []string, root string, ignore sourceIgnoreConfig) ([]string, error) {
	return sourcedomain.Listing(ctx, globs, sourcedomain.DiscoveryOptions{Root: root, IgnoreRoot: ignore.root, IgnorePaths: ignore.patterns, ProductionOnly: ignore.productionOnly})
}

func splitGlobRoots(globs []string) ([]string, []string) { return sourcedomain.SplitGlobRoots(globs) }
func withinRoot(path, root string) bool                  { return sourcedomain.WithinRoot(path, root) }

type gitignoreCache = sourcedomain.IgnoreCache

func pathIgnoredFromRoot(path, root string) bool { return sourcedomain.IgnoredFromRoot(path, root) }
func pathIgnoredFromRootCached(path, root string, cache gitignoreCache) bool {
	return sourcedomain.IgnoredFromRootCached(path, root, cache)
}

// PathMatchesGlobs is retained for Search API compatibility.
func PathMatchesGlobs(path string, patterns []string) bool {
	return sourcedomain.PathMatchesGlobs(path, patterns)
}
func pathMatchesGlobs(path string, patterns []string) bool {
	return sourcedomain.PathMatchesGlobs(path, patterns)
}
func globMatch(pattern, name string) bool { return sourcedomain.GlobMatch(pattern, name) }
func pathContainsGitDirectory(path string) bool {
	return sourcedomain.ContainsGitDirectory(path)
}
