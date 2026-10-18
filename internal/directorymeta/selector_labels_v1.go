package directorymeta

import (
	"path"
	"sort"
)

// SelectedLabelsV1 derives category unions for a matched subset of an existing
// inventory, without parsing metadata or hashing files again. Keys must be
// repository-relative selected file paths. No YAML directory claims are used.
func SelectedLabelsV1(inventory InventoryV1, matched map[string]bool) map[string]LabelsV1 {
	keys := make([]string, 0, len(matched))
	for key := range matched {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sets := map[string]*labelSetsV1{}
	for _, key := range keys {
		state, exists := inventory.Files[key]
		if !exists || state.Status != StatusCurrent {
			continue
		}
		for parent := path.Dir(key); ; parent = path.Dir(parent) {
			if sets[parent] == nil {
				sets[parent] = newLabelSetsV1()
			}
			sets[parent].add(state.File)
			if parent == "." {
				break
			}
		}
	}
	result := make(map[string]LabelsV1, len(sets))
	for key, value := range sets {
		result[key] = value.values()
	}
	return result
}

// FileLabelsV1 uses the same normalization/order as ancestor labels. A stale
// file's persisted tags must not appear in filtered tree output.
func FileLabelsV1(state FileStateV1) LabelsV1 {
	if state.Status != StatusCurrent {
		return LabelsV1{}
	}
	sets := newLabelSetsV1()
	sets.add(state.File)
	return sets.values()
}
