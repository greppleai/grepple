package api

import (
	"fmt"
	"github.com/greppleai/grepple/internal/search"
)

// NavigationSnapshotGroup is one semantically connected language group from a
// single checked immutable generation. All leases remain owned by the caller.
type NavigationSnapshotGroup struct {
	Languages []string
	Snapshot  NavigationSnapshot
}

// AttachNavigationGroups preserves result order while attaching each selected
// row using its coherent group. No complete graph or alternative resolver is built.
func AttachNavigationGroups(groups []NavigationSnapshotGroup, plan SearchPlan, batch SearchBatch, root, buildCWD string, observer NavigationObserver) (SearchBatch, error) {
	value, ok := batch.(searchBatch)
	if !ok || len(groups) == 0 || len(groups) > 64 {
		return nil, fmt.Errorf("invalid navigation groups")
	}
	owners, err := navigationGroupOwners(groups)
	if err != nil {
		return nil, err
	}
	if err := requireNavigationGroupOwners(batch, owners); err != nil {
		return nil, err
	}
	matches := append([]search.FileMatch(nil), value.matches...)
	for i, group := range groups {
		rows, selected := navigationGroupRows(value.matches, owners, i)
		if len(rows) == 0 {
			continue
		}
		attached, err := AttachNavigationSnapshotObserved(group.Snapshot, plan, searchBatch{matches: selected}, root, buildCWD, observer)
		if err != nil {
			return nil, err
		}
		result := attached.(searchBatch)
		for n, row := range rows {
			matches[row] = result.matches[n]
		}
	}
	return searchBatch{matches: matches}, nil
}
func navigationGroupOwners(groups []NavigationSnapshotGroup) (map[string]int, error) {
	owners := make(map[string]int)
	for i, group := range groups {
		if group.Snapshot == nil || len(group.Languages) == 0 {
			return nil, fmt.Errorf("invalid navigation group lease")
		}
		for _, language := range group.Languages {
			if _, exists := owners[language]; exists {
				return nil, fmt.Errorf("overlapping navigation groups")
			}
			owners[language] = i
		}
	}
	return owners, nil
}
func requireNavigationGroupOwners(batch SearchBatch, owners map[string]int) error {
	for _, language := range SearchBatchLanguages(batch) {
		if _, ok := owners[language]; !ok {
			return fmt.Errorf("selected navigation group unavailable")
		}
	}
	return nil
}
func navigationGroupRows(matches []search.FileMatch, owners map[string]int, index int) ([]int, []search.FileMatch) {
	var rows []int
	var selected []search.FileMatch
	for row, match := range matches {
		owner, ok := owners[match.Language]
		if ok && owner == index {
			rows = append(rows, row)
			selected = append(selected, match)
		}
	}
	return rows, selected
}
