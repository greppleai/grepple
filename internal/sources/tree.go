package sources

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// TreePath is a visible filesystem entry, not a claim about selected descendants.
// Depth-bounded trees include empty directories without inspecting their contents.
type TreePath struct {
	Path string
	Dir  bool
}

// ListTreeWithPolicy lists an unannotated checkout without reading directories
// below depth. It uses the same ignore matcher as recursive source discovery,
// including ancestor .gitignore files when scope is a checkout subdirectory.
// Callers must first establish that checkout has no .grepple metadata.
func ListTreeWithPolicy(ctx context.Context, scope, checkout string, depth int, policy Options) ([]TreePath, error) {
	return listTreeWithPolicy(ctx, scope, checkout, depth, policy, os.ReadDir)
}

func listTreeWithPolicy(ctx context.Context, scope, checkout string, depth int, policy Options, readDir func(string) ([]os.DirEntry, error)) ([]TreePath, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if depth < 1 {
		return nil, fmt.Errorf("tree depth must be positive")
	}
	scope, err := filepath.Abs(scope)
	if err != nil {
		return nil, err
	}
	checkout, err = filepath.Abs(checkout)
	if err != nil {
		return nil, err
	}
	if !withinRoot(scope, checkout) {
		return nil, fmt.Errorf("tree path %q is outside checkout %q", scope, checkout)
	}
	info, err := os.Stat(scope)
	if err != nil {
		return nil, err
	}
	ignore := ignoreConfig{root: policy.Root(), patterns: policy.IgnorePaths}
	result := []TreePath{}
	if containsGitDirectory(scope) || ignore.builtIn(scope) {
		return result, nil
	}
	if !info.IsDir() {
		// Explicit files keep the normal broad-policy bypass, without loading a
		// classifier: all kinds are already known to be unknown in this checkout.
		if !containsGitDirectory(scope) && !ignore.builtIn(scope) && !IsRepositoryMetadataSnapshot(scope) {
			result = append(result, TreePath{Path: scope})
			if policy.Notices != nil {
				reason := ""
				if ignore.filter().Ignored(scope) {
					reason = "grepple.json ignore.paths"
				} else if policy.ProductionOnly {
					reason = "--production-only"
				}
				if reason != "" {
					display, err := filepath.Rel(policy.ResolvedWorkingDirectory(), scope)
					if err != nil {
						display = scope
					}
					fmt.Fprintf(policy.Notices, "note: explicitly named file bypasses %s: %s\n", reason, filepath.ToSlash(display))
				}
			}
		}
		return result, nil
	}
	if policy.ProductionOnly {
		return result, nil // No annotated files can satisfy production policy.
	}
	w := candidateWalker{ctx: ctx, root: checkout, ignorePatterns: map[string][]string{}, ignore: ignore}
	// Load ancestor rules only, never sibling subtrees.
	var ancestors []string
	for directory := filepath.Dir(scope); scope != checkout && withinRoot(directory, checkout); directory = filepath.Dir(directory) {
		ancestors = append(ancestors, directory)
		if directory == checkout {
			break
		}
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		rel, _ := filepath.Rel(checkout, ancestors[i])
		if err := w.walkDir(ancestors[i], rel); err != nil {
			return result, nil
		}
	}
	var visit func(string, int) error
	visit = func(directory string, level int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, _ := filepath.Rel(checkout, directory)
		if err := w.walkDir(directory, rel); err != nil {
			return nil
		}
		entries, err := readDir(directory)
		if err != nil {
			return nil // Match source discovery's unreadable-subtree policy.
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			path := filepath.Join(directory, entry.Name())
			rel, _ := filepath.Rel(checkout, path)
			if entry.Name() == ".git" || ignored(rel, w.ignorePatterns) || ignore.builtIn(path) {
				continue
			}
			if entry.IsDir() {
				if ignore.ignoredDirectory(path) && !ignore.hasNegation() {
					continue
				}
				result = append(result, TreePath{Path: path, Dir: true})
				if level+1 < depth {
					if err := visit(path, level+1); err != nil {
						return err
					}
				}
			} else if !ignore.ignored(path) && !(entry.Type()&fs.ModeSymlink != 0 && IsRepositoryMetadataSnapshot(path)) {
				result = append(result, TreePath{Path: path})
			}
		}
		return nil
	}
	err = visit(scope, 0)
	return result, err
}
