package tree

import (
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/wire"
)

func buildUninitializedTree(path string, depth int, selector directorymeta.SelectorV1, repository cliruntime.Repository, working, checkout string) (wire.TreeResponse, bool, error) {
	working, err := filepath.Abs(working)
	if err != nil {
		return wire.TreeResponse{}, true, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return wire.TreeResponse{}, true, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return wire.TreeResponse{}, true, err
	}
	base := absolute
	if !info.IsDir() {
		base = filepath.Dir(base)
	}
	// Explicit paths in other or annotated nested checkouts retain their normal
	// behavior. Invocations outside a checkout never reach this fast path.
	if root, missing := directorymeta.UninitializedCheckout(base); !missing || root != checkout {
		return wire.TreeResponse{}, false, nil
	}
	if _, err := directorymeta.DirectoryKey(working, base); err != nil {
		return wire.TreeResponse{}, false, nil
	}
	policy := sources.Options{WorkingDirectory: working, IgnoreRoot: checkout}
	if repository != nil {
		policy, err = repository.ScopeOptions()
		if err != nil {
			return wire.TreeResponse{}, true, err
		}
	}
	display, err := filepath.Rel(working, absolute)
	if err != nil {
		return wire.TreeResponse{}, true, err
	}
	issues := []string{".grepple directory is missing from Git checkout"}
	response := wire.TreeResponse{Repo: ".", Path: filepath.ToSlash(display), Depth: depth,
		MetadataStatus: directorymeta.StatusMissing, MetadataIssues: issues, Entries: []wire.TreeEntry{}}
	if selector.Requested() {
		return response, true, nil // Missing annotations cannot satisfy selectors.
	}
	paths, err := sources.ListTreeWithPolicy(nil, absolute, checkout, depth, policy)
	if err != nil {
		return wire.TreeResponse{}, true, err
	}
	for _, entry := range paths {
		relative, err := filepath.Rel(base, entry.Path)
		if err != nil {
			return wire.TreeResponse{}, true, err
		}
		response.Entries = append(response.Entries, wire.TreeEntry{Path: filepath.ToSlash(relative), Dir: entry.Dir,
			MetadataStatus: directorymeta.StatusMissing, MetadataIssues: issues})
	}
	return response, true, nil
}
