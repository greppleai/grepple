package tree

import (
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/directorymeta"
)

// A tree invoked from a parent directory may contain independent checkouts.
// Their selected files must be checked against their own snapshots, not the
// parent's (possibly absent) .grepple/grepple.yaml. Keep keys relative to the
// invocation directory so depth, paths and selector unions remain unchanged.
func overlayNestedTreeMetadata(working string, files []string, inventory *directorymeta.InventoryV1) error {
	groups := map[string][]string{}
	for _, file := range files {
		absolute := file
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(working, filepath.FromSlash(file))
		}
		root, err := directorymeta.FindRoot(filepath.Dir(absolute))
		if err != nil {
			return err
		}
		if root == working {
			continue
		}
		// FindRoot returns the starting directory when no boundary exists.
		// Do not mistake every ordinary subdirectory for a nested checkout.
		if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
			if _, err := os.Stat(filepath.Join(root, ".grepple")); err != nil {
				continue
			}
		}
		prefix, err := directorymeta.DirectoryKey(working, root)
		if err != nil || prefix == "." {
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
