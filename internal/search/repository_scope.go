package search

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

var navigationRootMarkers = []string{filepath.Join(".grepple", "grepple.json"), filepath.Join(".grepple", "grepple.yaml"), ".git", "go.work"}

func collectRelatedRepositoryFiles(ctx context.Context, params Params, matches []FileMatch) ([]string, error) {
	var navigable []FileMatch
	for _, match := range matches {
		if supportsNavigation(match.Language) {
			navigable = append(navigable, match)
		}
	}
	if len(navigable) == 0 {
		return nil, nil
	}
	roots, err := relatedNavigationRoots(params.Root, navigable)
	if err != nil {
		return nil, err
	}
	ignore := sourceIgnoreConfig{root: params.IgnoreRoot, patterns: params.IgnorePaths, productionOnly: params.ProductionOnly}
	seen := make(map[string]bool)
	var files []string
	for _, root := range roots {
		candidates, collectErr := sourcedomain.Candidates(ctx, nil, sourcedomain.DiscoveryOptions{Root: root, IgnoreRoot: ignore.root, IgnorePaths: ignore.patterns, ProductionOnly: ignore.productionOnly})
		if collectErr != nil {
			return nil, collectErr
		}
		for _, candidate := range candidates {
			clean := filepath.Clean(candidate)
			if !seen[clean] {
				seen[clean] = true
				files = append(files, clean)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func relatedNavigationRoots(explicitRoot string, matches []FileMatch) ([]string, error) {
	if strings.TrimSpace(explicitRoot) != "" {
		root, err := filepath.Abs(explicitRoot)
		if err != nil {
			return nil, err
		}
		root = filepath.Clean(root)
		seen := make(map[string]bool)
		var roots []string
		for _, match := range matches {
			if match.File == "" || !withinRoot(match.File, root) {
				continue
			}
			selected, found := nearestNavigationRoot(filepath.Dir(match.File))
			if !found || !withinRoot(selected, root) {
				selected = root
			}
			if !seen[selected] {
				seen[selected] = true
				roots = append(roots, selected)
			}
		}
		if len(roots) == 0 {
			roots = append(roots, root)
		}
		sort.Strings(roots)
		return roots, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	starts := make([]string, 0, len(matches)+1)
	for _, match := range matches {
		if match.File != "" {
			starts = append(starts, filepath.Dir(match.File))
		}
	}
	if len(starts) == 0 {
		starts = append(starts, cwd)
	}
	seen := make(map[string]bool)
	roots := make([]string, 0, len(starts))
	for _, start := range starts {
		root, found := nearestNavigationRoot(start)
		if !found && withinRoot(start, cwd) {
			root = cwd
		}
		if !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	sort.Strings(roots)
	return roots, nil
}

func nearestNavigationRoot(start string) (string, bool) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return filepath.Clean(start), false
	}
	fallback := directory
	for {
		for _, marker := range navigationRootMarkers {
			if _, statErr := os.Stat(filepath.Join(directory, marker)); statErr == nil {
				return directory, true
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return fallback, false
		}
		directory = parent
	}
}
