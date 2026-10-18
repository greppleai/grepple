package tree

import (
	"fmt"
	"path/filepath"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/wire"
)

func buildLocalSelected(path string, depth int, selector directorymeta.SelectorV1, repository cliruntime.Repository) (wire.TreeResponse, error) {
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
	inventory, err := directorymeta.BuildInventoryV1(working, metadataFiles)
	if err != nil {
		return wire.TreeResponse{}, err
	}
	baseKey, err := directorymeta.DirectoryKey(working, base)
	if err != nil {
		return wire.TreeResponse{}, err
	}
	matching := selector.MatchingFiles(inventory)
	selected := make([]string, 0, len(files))
	matchedKeys := make(map[string]bool)
	for _, file := range files {
		key := metadataFileKey(file, working)
		if matching[key] {
			selected = append(selected, file)
			matchedKeys[key] = true
		}
	}
	union := directorymeta.SelectedLabelsV1(inventory, matchedKeys)
	entries := localEntriesV1(selected, base, working, depth, inventory)
	for index := range entries {
		entry := &entries[index]
		if entry.Dir {
			key, err := directorymeta.DirectoryKey(working, filepath.Join(base, filepath.FromSlash(entry.Path)))
			if err == nil {
				entry.Labels = treeLabelSummary(union[key])
			}
		} else {
			key := metadataFileKey(filepath.Join(base, filepath.FromSlash(entry.Path)), working)
			entry.Labels = treeLabelSummary(directorymeta.FileLabelsV1(inventory.Files[key]))
		}
	}
	display, err := filepath.Rel(working, absolute)
	if err != nil {
		return wire.TreeResponse{}, fmt.Errorf("display local tree path: %w", err)
	}
	inspection := inventory.Directories[baseKey]
	return wire.TreeResponse{Repo: ".", Path: filepath.ToSlash(display), Depth: depth, Entries: entries,
		Labels: treeLabelSummary(union[baseKey]), MetadataStatus: inspection.Status,
		MetadataIssues: append([]string(nil), inspection.Issues...)}, nil
}

// Both text and JSON expose at most two examples per category, plus exact totals.
func treeLabelSummary(labels directorymeta.LabelsV1) *wire.TreeLabels {
	result := &wire.TreeLabels{
		Purposes: treeLabelExamples(labels.Purposes), PurposeTotal: len(labels.Purposes),
		DomainAreas: treeLabelExamples(labels.DomainAreas), DomainAreaTotal: len(labels.DomainAreas),
		FunctionalAreas: treeLabelExamples(labels.FunctionalAreas), FunctionalAreaTotal: len(labels.FunctionalAreas),
		Keywords: treeLabelExamples(labels.Keywords), KeywordTotal: len(labels.Keywords),
	}
	if result.PurposeTotal+result.DomainAreaTotal+result.FunctionalAreaTotal+result.KeywordTotal == 0 {
		return nil
	}
	return result
}

func treeLabelExamples(values []string) []string {
	if len(values) > 2 {
		values = values[:2]
	}
	return append([]string(nil), values...)
}
