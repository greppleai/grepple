package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

func localTree(path string, depth int) (api.TreeResponse, error) {
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return api.TreeResponse{}, err
	}
	params := search.Params{Files: true, Globs: []string{path}}
	if err := applyRepositorySourceConfig(&params); err != nil {
		return api.TreeResponse{}, err
	}
	files, err := search.ListFilePaths(params, nil)
	if err != nil {
		return api.TreeResponse{}, err
	}
	working := mustGetwd()
	absolute, err := filepath.Abs(path)
	if err != nil {
		return api.TreeResponse{}, err
	}
	base := absolute
	if !info.IsDir() {
		base = filepath.Dir(absolute)
	}
	result := localTreeEntries(files, base, working, depth)
	display, err := filepath.Rel(working, absolute)
	if err != nil {
		return api.TreeResponse{}, fmt.Errorf("display local tree path: %w", err)
	}
	return api.TreeResponse{Repo: ".", Path: filepath.ToSlash(display), Depth: depth, Entries: result}, nil
}

func localTreeEntries(files []string, base, working string, depth int) []api.TreeEntry {
	entries := map[string]bool{}
	for _, file := range files {
		appendLocalTreePath(entries, file, base, working, depth)
	}
	result := make([]api.TreeEntry, 0, len(entries))
	for entryPath, directory := range entries {
		result = append(result, api.TreeEntry{Path: entryPath, Dir: directory})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Dir && !result[j].Dir
	})
	return result
}

func appendLocalTreePath(entries map[string]bool, file, base, working string, depth int) {
	filePath := file
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(working, filepath.FromSlash(filePath))
	}
	relative, err := filepath.Rel(base, filePath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index := 0; index < len(parts) && index < depth; index++ {
		entryPath := strings.Join(parts[:index+1], "/")
		entries[entryPath] = entries[entryPath] || index < len(parts)-1
	}
}
