package search

import (
	"sort"

	"github.com/greppleai/grepple/parser"
)

// NavigationResolutionCount is one deterministic label/count pair.
type NavigationResolutionCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// NavigationLanguageResolutionStats measures call resolution for one language family.
type NavigationLanguageResolutionStats struct {
	Language      string                      `json:"language"`
	Calls         int                         `json:"calls"`
	Resolved      int                         `json:"resolved"`
	Ambiguous     int                         `json:"ambiguous"`
	Unresolved    int                         `json:"unresolved"`
	Candidate     int                         `json:"candidate"`
	AmbiguityRate float64                     `json:"ambiguityRate"`
	Confidences   []NavigationResolutionCount `json:"confidences"`
}

// NavigationResolutionStats reports deterministic ambiguity frequency and confidence counts.
type NavigationResolutionStats struct {
	Calls         int                                 `json:"calls"`
	Resolved      int                                 `json:"resolved"`
	Ambiguous     int                                 `json:"ambiguous"`
	Unresolved    int                                 `json:"unresolved"`
	Candidate     int                                 `json:"candidate"`
	AmbiguityRate float64                             `json:"ambiguityRate"`
	Confidences   []NavigationResolutionCount         `json:"confidences"`
	Languages     []NavigationLanguageResolutionStats `json:"languages"`
}

// MeasureNavigationResolution classifies every graph call without changing graph facts.
func MeasureNavigationResolution(graph parser.NavigationGraph) NavigationResolutionStats {
	total := navigationResolutionAccumulator{confidences: make(map[string]int)}
	languages := make(map[string]*navigationResolutionAccumulator)
	for _, call := range graph.Calls {
		language := navigationLanguageFamily(call.Language)
		if languages[language] == nil {
			languages[language] = &navigationResolutionAccumulator{confidences: make(map[string]int)}
		}
		accumulateNavigationResolution(&total, call)
		accumulateNavigationResolution(languages[language], call)
	}
	result := total.stats()
	languageNames := make([]string, 0, len(languages))
	for language := range languages {
		languageNames = append(languageNames, language)
	}
	sort.Strings(languageNames)
	for _, language := range languageNames {
		stats := languages[language].stats()
		result.Languages = append(result.Languages, NavigationLanguageResolutionStats{Language: language, Calls: stats.Calls, Resolved: stats.Resolved, Ambiguous: stats.Ambiguous, Unresolved: stats.Unresolved, Candidate: stats.Candidate, AmbiguityRate: stats.AmbiguityRate, Confidences: stats.Confidences})
	}
	return result
}

type navigationResolutionAccumulator struct {
	calls, resolved, ambiguous, unresolved, candidate int
	confidences                                       map[string]int
}

func accumulateNavigationResolution(stats *navigationResolutionAccumulator, call parser.NavigationCall) {
	stats.calls++
	stats.confidences[call.Confidence]++
	switch {
	case call.TargetID != "":
		stats.resolved++
	case len(call.CandidateTargetIDs) > 1:
		stats.ambiguous++
	case len(call.CandidateTargetIDs) == 1:
		stats.candidate++
	default:
		stats.unresolved++
	}
}

func (stats navigationResolutionAccumulator) stats() NavigationResolutionStats {
	result := NavigationResolutionStats{Calls: stats.calls, Resolved: stats.resolved, Ambiguous: stats.ambiguous, Unresolved: stats.unresolved, Candidate: stats.candidate, Confidences: navigationResolutionCounts(stats.confidences), Languages: []NavigationLanguageResolutionStats{}}
	if stats.calls > 0 {
		result.AmbiguityRate = float64(stats.ambiguous) / float64(stats.calls)
	}
	return result
}

func navigationResolutionCounts(counts map[string]int) []NavigationResolutionCount {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]NavigationResolutionCount, 0, len(names))
	for _, name := range names {
		result = append(result, NavigationResolutionCount{Name: name, Count: counts[name]})
	}
	return result
}
