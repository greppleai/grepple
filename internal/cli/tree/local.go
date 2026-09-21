package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

// SourceConfigurator applies repository source-selection policy.
type SourceConfigurator func(*search.Params) error

// NewLocal constructs local tree inspection from source policy and working-directory services.
func NewLocal(configure SourceConfigurator, workingDirectory func() string) LocalTree {
	return func(path string, depth int) (api.TreeResponse, error) {
		return buildLocal(path, depth, configure, workingDirectory)
	}
}

func buildLocal(path string, depth int, configure SourceConfigurator, workingDirectory func() string) (api.TreeResponse, error) {
	if path == "" {
		path = "."
	}
	info, files, err := localSourcePaths(path, configure)
	if err != nil {
		return api.TreeResponse{}, err
	}
	working := "."
	if workingDirectory != nil {
		working = workingDirectory()
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return api.TreeResponse{}, err
	}
	base := absolute
	if !info.IsDir() {
		base = filepath.Dir(absolute)
	}
	result := localEntries(files, base, working, depth)
	display, err := filepath.Rel(working, absolute)
	if err != nil {
		return api.TreeResponse{}, fmt.Errorf("display local tree path: %w", err)
	}
	return api.TreeResponse{Repo: ".", Path: filepath.ToSlash(display), Depth: depth, Entries: result}, nil
}

func localSourcePaths(path string, configure SourceConfigurator) (os.FileInfo, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	params := search.Params{Files: true, Globs: []string{path}}
	if configure != nil {
		if err := configure(&params); err != nil {
			return nil, nil, err
		}
	}
	files, err := search.ListFilePaths(params, nil)
	return info, files, err
}

func localEntries(files []string, base, working string, depth int) []api.TreeEntry {
	entries := map[string]bool{}
	for _, file := range files {
		appendLocalPath(entries, file, base, working, depth)
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

func appendLocalPath(entries map[string]bool, file, base, working string, depth int) {
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
