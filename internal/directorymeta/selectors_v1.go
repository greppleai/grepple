package directorymeta

import "fmt"

// SelectorsV1 describes local metadata predicates. Repeated values within a
// category are ORed; categories are ANDed on the same checksum-current file.
type SelectorsV1 struct {
	Purposes        []string
	DomainAreas     []string
	FunctionalAreas []string
	Keywords        []string
}

type SelectorV1 struct {
	purposes, domain, functional, keywords map[string]bool
	requested                              bool
}

// CompileSelectorsV1 rejects invalid labels before source retrieval. Metadata
// keys are normalized exact labels, not substring or fuzzy matches.
func CompileSelectorsV1(values SelectorsV1) (SelectorV1, error) {
	result := SelectorV1{}
	for _, category := range []struct {
		flag   string
		values []string
		dest   *map[string]bool
	}{
		{"--purpose", values.Purposes, &result.purposes},
		{"--domain-area", values.DomainAreas, &result.domain},
		{"--functional-area", values.FunctionalAreas, &result.functional},
		{"--keyword", values.Keywords, &result.keywords},
	} {
		if len(category.values) == 0 {
			continue
		}
		result.requested = true
		*category.dest = make(map[string]bool, len(category.values))
		for _, value := range category.values {
			if !ValidLabelV1(value) {
				return SelectorV1{}, fmt.Errorf("invalid %s %q: expected a nonempty label of at most 80 UTF-8 bytes without control characters", category.flag, value)
			}
			(*category.dest)[LabelKey(value)] = true
		}
	}
	return result, nil
}

func (selector SelectorV1) Requested() bool { return selector.requested }

func matchesOneV1(wanted map[string]bool, values []string) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, value := range values {
		if wanted[LabelKey(value)] {
			return true
		}
	}
	return false
}

// Matches accepts ordinary unfiltered selected files, but never trusts missing,
// stale or invalid annotations when any metadata predicate is requested.
func (selector SelectorV1) Matches(state FileStateV1) bool {
	if !selector.requested {
		return true
	}
	if state.Status != StatusCurrent {
		return false
	}
	file := state.File
	return matchesOneV1(selector.purposes, []string{file.Purpose}) &&
		matchesOneV1(selector.domain, file.DomainAreas) &&
		matchesOneV1(selector.functional, file.FunctionalAreas) &&
		matchesOneV1(selector.keywords, file.Keywords)
}

// MatchingFiles accepts only keys already present in the caller's source-scoped
// inventory; a YAML entry can never expand the selected universe.
func (selector SelectorV1) MatchingFiles(inventory InventoryV1) map[string]bool {
	result := make(map[string]bool)
	for path, state := range inventory.Files {
		if selector.Matches(state) {
			result[path] = true
		}
	}
	return result
}
