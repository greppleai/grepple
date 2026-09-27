package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func buildLocal(path string, depth int, kind sourcedomain.Kind, selectedAreas []string, repository cliruntime.Repository) (wire.TreeResponse, error) {

	if path == "" {
		path = "."
	}
	info, files, err := localSourcePaths(path, repository)
	if err != nil {
		return wire.TreeResponse{}, err
	}
	working := "."
	if repository != nil {
		working = repository.WorkingDirectory()
	}
	working, err = filepath.Abs(working)
	if err != nil {
		return wire.TreeResponse{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return wire.TreeResponse{}, err
	}
	base := absolute
	metadataFiles := files
	if !info.IsDir() {
		base = filepath.Dir(absolute)
		_, metadataFiles, err = localSourcePaths(base, repository)
		if err != nil {
			return wire.TreeResponse{}, err
		}
	}
	inspection := directorymeta.Inspect(working, base, metadataFiles)
	if kind != "" {
		if repository != nil {
			policy, err := repository.ScopeOptions()
			if err != nil {
				return wire.TreeResponse{}, err
			}
			if policy.ProductionOnly && kind != sourcedomain.Production {
				return wire.TreeResponse{}, fmt.Errorf("--kind %s cannot be combined with --production-only", kind)
			}
		}
		classifier := sourcedomain.NewClassifier(working)
		selected := files[:0:0]
		for _, file := range files {
			if classifier.Classify(file) == kind {
				selected = append(selected, file)
			}
		}
		files = selected
	}
	areaReferences, err := directorymeta.AreaIndex(working, files)
	if err != nil {
		return wire.TreeResponse{}, err
	}
	if len(selectedAreas) > 0 {
		files, areaReferences = filterLocalAreas(files, areaReferences, working, selectedAreas)
	}
	areas := localAreaMembership(areaReferences, base, working)
	result := localEntries(files, metadataFiles, base, working, depth, areas)
	display, err := filepath.Rel(working, absolute)
	if err != nil {
		return wire.TreeResponse{}, fmt.Errorf("display local tree path: %w", err)
	}
	response := wire.TreeResponse{
		Repo:           ".",
		Path:           filepath.ToSlash(display),
		Areas:          areas["."],
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

// filterLocalAreas keeps current files matching any requested tag. Other tags
// on those same files remain visible to explain the areas each result touches.
func filterLocalAreas(files []string, references []directorymeta.AreaReference, working string, requested []string) ([]string, []directorymeta.AreaReference) {
	wanted := make(map[string]bool, len(requested))
	for _, area := range requested {
		wanted[area] = true
	}
	matched := map[string]bool{}
	for _, reference := range references {
		if reference.Status == directorymeta.StatusCurrent && wanted[reference.Area] {
			matched[reference.Path] = true
		}
	}
	selected := make([]string, 0, len(files))
	for _, file := range files {
		absolute := file
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(working, filepath.FromSlash(file))
		}
		relative, err := filepath.Rel(working, absolute)
		if err == nil && matched[filepath.ToSlash(relative)] {
			selected = append(selected, file)
		}
	}
	visible := make([]directorymeta.AreaReference, 0, len(references))
	for _, reference := range references {
		if matched[reference.Path] {
			visible = append(visible, reference)
		}
	}
	return selected, visible
}

func localEntries(files, metadataFiles []string, base, working string, depth int, areas map[string][]string) []wire.TreeEntry {
	entries := map[string]bool{}
	metadataCache := map[string]directorymeta.Inspection{}
	for _, file := range files {
		appendLocalPath(entries, file, base, working, depth)
	}
	result := make([]wire.TreeEntry, 0, len(entries))
	for entryPath, directory := range entries {
		entry := wire.TreeEntry{Path: entryPath, Dir: directory, Areas: areas[entryPath]}
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

// localAreaMembership rolls fresh selected-file tags up to every ancestor,
// including directories hidden by a shallow tree depth. Stale tags stay review
// leads in metadata, never authoritative area labels in tree output.
func localAreaMembership(references []directorymeta.AreaReference, base, working string) map[string][]string {
	sets := map[string]map[string]bool{}
	for _, reference := range references {
		if reference.Status != directorymeta.StatusCurrent || !directorymeta.ValidArea(reference.Area) {
			continue
		}
		path := filepath.Join(working, filepath.FromSlash(reference.Path))
		relative, err := filepath.Rel(base, path)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		for index := 0; index <= len(parts); index++ {
			entry := "."
			if index > 0 {
				entry = strings.Join(parts[:index], "/")
			}
			if sets[entry] == nil {
				sets[entry] = map[string]bool{}
			}
			sets[entry][reference.Area] = true
		}
	}
	areas := make(map[string][]string, len(sets))
	for path, values := range sets {
		for area := range values {
			areas[path] = append(areas[path], area)
		}
		sort.Strings(areas[path])
	}
	return areas
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
