package tree

import (
	"os"
	"path/filepath"
)

// nestedTreeRoots resolves only boundaries below the invocation directory.
// An empty cached root means there is no nested checkout, not that the
// directory is itself a metadata root (FindRoot's uninitialized fallback).
// The cache is invocation-local so later commands observe new markers.
type nestedTreeRoots struct {
	working string
	cache   map[string]string
}

func (roots *nestedTreeRoots) find(directory string) string {
	var visited []string
	root := ""
	for {
		if cached, ok := roots.cache[directory]; ok {
			root = cached
			break
		}
		visited = append(visited, directory)
		// .grepple/grepple.yaml cannot exist without the .grepple boundary.
		for _, marker := range []string{".grepple", ".git"} {
			if _, err := os.Stat(filepath.Join(directory, marker)); err == nil {
				root = directory
				break
			}
		}
		if root != "" || directory == roots.working || filepath.Dir(directory) == directory {
			break
		}
		directory = filepath.Dir(directory)
	}
	for _, directory := range visited {
		roots.cache[directory] = root
	}
	return root
}
