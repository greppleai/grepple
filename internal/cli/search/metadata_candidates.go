package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/search"
)

// selectLocalMetadataCandidates intersects the actual mode-specific source
// universe with checksum-current annotations. Never substitute metadata paths
// for source-selected paths; even an empty result is a non-nil candidate list.
func selectLocalMetadataCandidates(ctx context.Context, options *Options) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, err := directorymeta.FindRoot(cwd)
	if err != nil {
		return nil, err
	}
	paths, err := search.DiscoverLocalCandidatesContext(ctx, options.Params, options.Outline || options.Params.Files)
	if err != nil {
		return nil, err
	}
	confined := make([]string, 0, len(paths))
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			continue
		}
		confined = append(confined, absolute)
	}
	inventory, err := directorymeta.BuildInventoryV1(root, confined)
	if err != nil {
		return nil, fmt.Errorf("inspect local metadata: %w", err)
	}
	matching := options.MetadataSelector.MatchingFiles(inventory)
	selected := make([]string, 0, len(confined))
	for _, path := range confined {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		if matching[filepath.ToSlash(relative)] {
			selected = append(selected, path)
		}
	}
	return selected, nil
}
