package search

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var navigationRootMarkers = []string{"grepple.json", ".git", "go.work"}

func collectRelatedRepositoryFiles(ctx context.Context, params Params, matches []FileMatch) ([]string, error) {
	roots, err := relatedNavigationRoots(params.Root, matches)
	if err != nil {
		return nil, err
	}
	ignore := sourceIgnoreConfig{root: params.IgnoreRoot, patterns: params.IgnorePaths, productionOnly: params.ProductionOnly}
	seen := make(map[string]bool)
	var files []string
	for _, root := range roots {
		candidates, collectErr := collectCandidateFilesConfiguredContext(ctx, nil, root, ignore)
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
		return []string{filepath.Clean(root)}, nil
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
