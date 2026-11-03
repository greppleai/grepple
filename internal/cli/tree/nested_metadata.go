package tree

import (
	"path/filepath"

	"github.com/greppleai/grepple/internal/directorymeta"
)

// A tree invoked from a parent directory may contain independent checkouts.
// Their selected files must be checked against their own snapshots, not the
// parent's (possibly absent) .grepple/grepple.yaml. Keep keys relative to the
// invocation directory so depth, paths and selector unions remain unchanged.
func overlayNestedTreeMetadata(working string, files []string, inventory *directorymeta.InventoryV1) error {
	groups := map[string][]string{}
	// Cache both found boundaries and misses for the entire invocation. Most
	// files share directories; probing their ancestors per file dominates tree
	// latency on filesystems with expensive metadata operations.
	roots := nestedTreeRoots{working: working, cache: map[string]string{working: ""}}
	for _, file := range files {
		absolute := file
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(working, filepath.FromSlash(file))
		}
		if _, err := directorymeta.DirectoryKey(working, filepath.Dir(absolute)); err != nil {
			continue
		}
		root := roots.find(filepath.Dir(absolute))
		if root == "" {
			continue
		}
		groups[root] = append(groups[root], absolute)
	}
	for root, selected := range groups {
		nested, err := directorymeta.BuildInventoryV1(root, selected)
		if err != nil {
			return err
		}
		prefix, _ := directorymeta.DirectoryKey(working, root)
		for key, state := range nested.Files {
			inventory.Files[treeNestedKey(prefix, key)] = state
		}
		for key, state := range nested.Directories {
			inventory.Directories[treeNestedKey(prefix, key)] = state
		}
	}
	return nil
}

func treeNestedKey(prefix, key string) string {
	if key == "." {
		return prefix
	}
	return prefix + "/" + key
}
