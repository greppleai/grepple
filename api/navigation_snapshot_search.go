package api

import (
	"fmt"
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
)

// SearchBatchPaths returns physical selected files, not repository discovery.
func SearchBatchPaths(batch SearchBatch) []string {
	value, ok := batch.(searchBatch)
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(value.matches))
	service := parser.NewParser()
	for _, match := range value.matches {
		capabilities, ok := service.CapabilitiesForLanguage(match.Language)
		if ok && capabilities.Navigation {
			paths = append(paths, match.File)
		}
	}
	return paths
}
func (snapshot *navigationSnapshot) Attach(plan SearchPlan, batch SearchBatch, root, buildCWD string) (SearchBatch, error) {
	return snapshot.attach(plan, batch, root, buildCWD, nil)
}
func (snapshot *navigationSnapshot) attach(plan SearchPlan, batch SearchBatch, root, buildCWD string, observer navigation.SnapshotObserver) (SearchBatch, error) {
	value, ok := batch.(searchBatch)
	if !ok {
		return nil, fmt.Errorf("unsupported search batch")
	}
	matches := append([]search.FileMatch(nil), value.matches...)
	options := plan.Options()
	if err := search.AttachStoredNavigation(matches, snapshot.snapshot, root, buildCWD, options.FollowRelated, options.At != "", observer); err != nil {
		return nil, err
	}
	return searchBatch{matches: matches}, nil
}
