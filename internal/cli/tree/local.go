package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func buildLocal(path string, depth int, repository cliruntime.Repository) (api.TreeResponse, error) {

	if path == "" {
		path = "."
	}
	info, files, err := localSourcePaths(path, repository)
	if err != nil {
		return api.TreeResponse{}, err
	}
	working := "."
	if repository != nil {
		working = repository.WorkingDirectory()
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return api.TreeResponse{}, err
	}
	base := absolute
	metadataFiles := files
	if !info.IsDir() {
		base = filepath.Dir(absolute)
		_, metadataFiles, err = localSourcePaths(base, repository)
		if err != nil {
			return api.TreeResponse{}, err
		}
	}
	inspection := directorymeta.Inspect(working, base, metadataFiles)
	result := localEntries(files, metadataFiles, base, working, depth)
	display, err := filepath.Rel(working, absolute)
	if err != nil {
		return api.TreeResponse{}, fmt.Errorf("display local tree path: %w", err)
	}
	response := api.TreeResponse{
		Repo:           ".",
		Path:           filepath.ToSlash(display),
		Depth:          depth,
		Entries:        result,
		Description:    inspection.Metadata.Description,
		MetadataStatus: inspection.Status,
		MetadataIssues: append([]string(nil), inspection.Issues...),
	}
	return response, nil
}

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

func localEntries(files, metadataFiles []string, base, working string, depth int) []api.TreeEntry {
	entries := map[string]bool{}
	metadataCache := map[string]directorymeta.Inspection{}
	for _, file := range files {
		appendLocalPath(entries, file, base, working, depth)
	}
	result := make([]api.TreeEntry, 0, len(entries))
	for entryPath, directory := range entries {
		entry := api.TreeEntry{Path: entryPath, Dir: directory}
		entry.Description, entry.MetadataStatus, entry.MetadataIssues = localEntryMetadata(base, working, entryPath, directory, metadataFiles, metadataCache)
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
func localEntryMetadata(base, working, entryPath string, directory bool, files []string, cache map[string]directorymeta.Inspection) (string, string, []string) {
	metadataDirectory := filepath.Join(base, filepath.FromSlash(entryPath))
	if !directory {
		metadataDirectory = filepath.Dir(metadataDirectory)
	}
	metadataDirectory = filepath.Clean(metadataDirectory)
	inspection, found := cache[metadataDirectory]
	if !found {
		inspection = directorymeta.Inspect(working, metadataDirectory, files)
		cache[metadataDirectory] = inspection
	}
	if directory {
		return inspection.Metadata.Description, inspection.Status, append([]string(nil), inspection.Issues...)
	}
	name := filepath.Base(filepath.FromSlash(entryPath))
	if name == directorymeta.FileName {
		return "Describes this directory's responsibilities and files for Grepple.", inspection.Status, append([]string(nil), inspection.Issues...)
	}
	state, exists := inspection.Files[name]
	if !exists {
		return "", directorymeta.StatusMissing, []string{"file entry is missing from grepple.yaml"}
	}
	return state.Description, state.Status, append([]string(nil), state.Issues...)
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
