package search

import (
	"context"
	"encoding/json"
)

// The immutable revision covers discovery/context files as well as graph sources.
// This lets warm complete-repository lookups avoid walking and rereading the tree.
func attachImmutableRepositoryNavigation(ctx context.Context, params Params, matches []FileMatch, scan candidateScan) error {
	var navigable []FileMatch
	for _, match := range matches {
		if supportsNavigation(match.Language) {
			navigable = append(navigable, match)
		}
	}
	roots, err := relatedNavigationRoots(params.Root, navigable)
	if err != nil {
		return err
	}
	key := immutableRepositoryNavigationKey(params, roots)
	index, err := immutableRelatedLookups.resolve(ctx, key, func() (*navigationIndex, error) {
		candidates, err := collectRelatedRepositoryFiles(ctx, params, navigable)
		if err != nil {
			return nil, err
		}
		if params.RelatedRepositoryContext {
			scan.fromIndex = false
		}
		return buildNavigationIndex(scan.relatedFiles(candidates), true), nil
	})
	if err != nil {
		return err
	}
	attachRelatedNavigation(matches, index, params.FollowRelated, false)
	return nil
}
func immutableRepositoryNavigationKey(params Params, roots []string) string {
	// Ordered ignore rules and repository filters are part of the universe identity.
	options, _ := json.Marshal([]any{params.Root, params.IgnoreRoot, params.IgnorePaths, params.ProductionOnly, params.Repo, params.ExcludeRepo})
	return immutableNavigationKey("repository-scope\x00"+params.ImmutableNavigationRevision+"\x00"+string(options), roots)
}
