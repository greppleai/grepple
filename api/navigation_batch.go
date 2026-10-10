package api

import (
	"fmt"
	"github.com/greppleai/grepple/internal/search"
	"path/filepath"
)

// SelectNavigationBatchPaths creates a request-owned batch for checked owners.
// Source strings are immutable and borrowed; result mutations remain isolated.
func SelectNavigationBatchPaths(batch SearchBatch, paths []string) (SearchBatch, error) {
	value, ok := batch.(searchBatch)
	if !ok {
		return nil, fmt.Errorf("unsupported search batch")
	}
	wanted := map[string]bool{}
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		wanted[absolute] = true
	}
	var matches []search.FileMatch
	for _, match := range value.matches {
		absolute, err := filepath.Abs(match.File)
		if err != nil {
			return nil, err
		}
		if wanted[absolute] {
			matches = append(matches, match)
		}
	}
	return searchBatch{matches: matches}, nil
}

// MergeNavigationBatches merges only navigation fields from disjoint owned rows.
// Original content, matching, structural facts, ordering and pagination survive.
func MergeNavigationBatches(batch SearchBatch, parts []SearchBatch) (SearchBatch, error) {
	value, ok := batch.(searchBatch)
	if !ok {
		return nil, fmt.Errorf("unsupported search batch")
	}
	matches := append([]search.FileMatch(nil), value.matches...)
	owners := map[string]int{}
	for row, match := range matches {
		if _, exists := owners[match.File]; exists {
			return nil, fmt.Errorf("duplicate navigation result path")
		}
		owners[match.File] = row
	}
	seen := map[string]bool{}
	for _, part := range parts {
		selected, ok := part.(searchBatch)
		if !ok {
			return nil, fmt.Errorf("unsupported navigation batch")
		}
		for _, match := range selected.matches {
			row, ok := owners[match.File]
			if !ok || seen[match.File] || match.Language != matches[row].Language {
				return nil, fmt.Errorf("invalid navigation result ownership")
			}
			seen[match.File] = true
			matches[row].Related = match.Related
			matches[row].OmittedRelatedCallers = match.OmittedRelatedCallers
			matches[row].OmittedRelatedCallees = match.OmittedRelatedCallees
			matches[row].OmittedRelatedTypes = match.OmittedRelatedTypes
		}
	}
	return searchBatch{matches: matches}, nil
}
