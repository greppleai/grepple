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

// DiscoverLocalCandidatesContext uses the same source policy and glob discovery
// as the requested search mode, before metadata is evaluated or paging begins.
func DiscoverLocalCandidatesContext(ctx context.Context, p Params, listing bool) ([]string, error) {
	globs := p.Globs
	if p.At != "" {
		path, _, _, err := atReference(p)
		if err != nil {
			return nil, err
		}
		globs = []string{path}
	}
	ignore := sourceIgnoreConfig{root: p.IgnoreRoot, patterns: p.IgnorePaths, productionOnly: p.ProductionOnly}
	if listing {
		return collectListingFilesConfiguredContext(ctx, globs, p.Root, ignore)
	}
	return collectCandidateFilesConfiguredContext(ctx, globs, p.Root, ignore)
}

func collectCandidateFilesConfiguredContext(ctx context.Context, globs []string, root string, ignore sourceIgnoreConfig) ([]string, error) {
	return sourcedomain.Candidates(ctx, globs, sourcedomain.DiscoveryOptions{Root: root, IgnoreRoot: ignore.root, IgnorePaths: ignore.patterns, ProductionOnly: ignore.productionOnly})
}

func collectListingFiles(globs []string, root string) ([]string, error) {
	return sourcedomain.Listing(context.Background(), globs, sourcedomain.DiscoveryOptions{Root: root})
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
