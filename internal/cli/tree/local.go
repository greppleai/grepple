package tree

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/wire"
)

func localSourcePaths(path string, repository cliruntime.Repository) (os.FileInfo, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	policy := sourcedomain.Options{}
	if repository != nil {
		policy, err = repository.ScopeOptions()
		if err != nil {
			return nil, nil, err
		}
	}
	files, err := sourcedomain.ListWithPolicy(nil, []string{path}, "", policy)
	return info, files, err
}

// metadataFileKey returns a slash-separated path relative to the repository.
func metadataFileKey(path, working string) string {
	absolute := path
	if !filepath.IsAbs(path) {
		absolute = filepath.Join(working, filepath.FromSlash(path))
	}
	relative, err := filepath.Rel(working, absolute)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(relative)
}

func localEntriesV1(files []string, base, working string, depth int, inventory directorymeta.InventoryV1) []wire.TreeEntry {
	entries := map[string]bool{}
	for _, file := range files {
		appendLocalPath(entries, file, base, working, depth)
	}
	result := make([]wire.TreeEntry, 0, len(entries))
	for entryPath, directory := range entries {
		entry := wire.TreeEntry{Path: entryPath, Dir: directory}
		if directory {
			key, err := directorymeta.DirectoryKey(working, filepath.Join(base, filepath.FromSlash(entryPath)))
			if err == nil {
				inspection := inventory.Directories[key]
				entry.MetadataStatus = inspection.Status
				entry.MetadataIssues = append([]string(nil), inspection.Issues...)
			}
		} else {
			key := metadataFileKey(filepath.Join(base, filepath.FromSlash(entryPath)), working)
			state, exists := inventory.Files[key]
			if !exists {
				entry.MetadataStatus = directorymeta.StatusMissing
				entry.MetadataIssues = []string{"file entry is missing from .grepple/grepple.yaml"}
			} else {
				entry.Description = state.File.Description
				entry.MetadataStatus = state.Status
				entry.MetadataIssues = append([]string(nil), state.Issues...)
			}
		}
		result = append(result, entry)
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
