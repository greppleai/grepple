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

// NavigationLanguageResolutionStats measures call outcomes and confidence evidence for one language family.
type NavigationLanguageResolutionStats struct {
	Language             string                      `json:"language"`
	Calls                int                         `json:"calls"`
	Resolved             int                         `json:"resolved"`
	Ambiguous            int                         `json:"ambiguous"`
	Unresolved           int                         `json:"unresolved"`
	Candidate            int                         `json:"candidate"`
	AmbiguityRate        float64                     `json:"ambiguityRate"`
	ResolvedLocal        int                         `json:"resolvedLocal"`
	AmbiguousLocal       int                         `json:"ambiguousLocal"`
	UnresolvedLocal      int                         `json:"unresolvedLocal"`
	ExpectedExternal     int                         `json:"expectedExternal"`
	ResolutionRate       float64                     `json:"resolutionRate"`
	AmbiguousLocalRate   float64                     `json:"ambiguousLocalRate"`
	UnresolvedLocalRate  float64                     `json:"unresolvedLocalRate"`
	ExpectedExternalRate float64                     `json:"expectedExternalRate"`
	Outcomes             []NavigationResolutionCount `json:"outcomes"`
	Confidences          []NavigationResolutionCount `json:"confidences"`
}

// NavigationResolutionStats reports resolution outcomes separately from confidence evidence.
// Resolved, Ambiguous, Unresolved, Candidate, and AmbiguityRate retain their
// legacy cardinality semantics for compatible JSON consumers.
type NavigationResolutionStats struct {
	Calls                int                                 `json:"calls"`
	Resolved             int                                 `json:"resolved"`
	Ambiguous            int                                 `json:"ambiguous"`
	Unresolved           int                                 `json:"unresolved"`
	Candidate            int                                 `json:"candidate"`
	AmbiguityRate        float64                             `json:"ambiguityRate"`
	ResolvedLocal        int                                 `json:"resolvedLocal"`
	AmbiguousLocal       int                                 `json:"ambiguousLocal"`
	UnresolvedLocal      int                                 `json:"unresolvedLocal"`
	ExpectedExternal     int                                 `json:"expectedExternal"`
	ResolutionRate       float64                             `json:"resolutionRate"`
	AmbiguousLocalRate   float64                             `json:"ambiguousLocalRate"`
	UnresolvedLocalRate  float64                             `json:"unresolvedLocalRate"`
	ExpectedExternalRate float64                             `json:"expectedExternalRate"`
	Outcomes             []NavigationResolutionCount         `json:"outcomes"`
	Confidences          []NavigationResolutionCount         `json:"confidences"`
	Languages            []NavigationLanguageResolutionStats `json:"languages"`
}

// MeasureNavigationResolution classifies every graph call without changing graph facts.
func MeasureNavigationResolution(graph parser.NavigationGraph) NavigationResolutionStats {
	total := newNavigationResolutionAccumulator()
	languages := make(map[string]*navigationResolutionAccumulator)
	for _, call := range graph.Calls {
		language := navigationLanguageFamily(call.Language)
		if languages[language] == nil {
			languages[language] = newNavigationResolutionAccumulator()
		}
		accumulateNavigationResolution(total, call)
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
		result.Languages = append(result.Languages, navigationLanguageResolutionStats(language, stats))
	}
	return result
}

type navigationResolutionAccumulator struct {
	calls, resolved, ambiguous, unresolved, candidate                int
	resolvedLocal, ambiguousLocal, unresolvedLocal, expectedExternal int
	confidences, outcomes                                            map[string]int
}

func newNavigationResolutionAccumulator() *navigationResolutionAccumulator {
	return &navigationResolutionAccumulator{confidences: make(map[string]int), outcomes: make(map[string]int)}
}

func accumulateNavigationResolution(stats *navigationResolutionAccumulator, call parser.NavigationCall) {
	stats.calls++
	stats.confidences[call.Confidence]++
	outcome := navigationResolutionOutcome(call)
	stats.outcomes[outcome]++
	switch outcome {
	case "resolved-local":
		stats.resolvedLocal++
	case "ambiguous-local":
		stats.ambiguousLocal++
	case "expected-external":
		stats.expectedExternal++
	default:
		stats.unresolvedLocal++
	}
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
	result := NavigationResolutionStats{
		Calls: stats.calls, Resolved: stats.resolved, Ambiguous: stats.ambiguous, Unresolved: stats.unresolved, Candidate: stats.candidate,
		ResolvedLocal: stats.resolvedLocal, AmbiguousLocal: stats.ambiguousLocal, UnresolvedLocal: stats.unresolvedLocal, ExpectedExternal: stats.expectedExternal,
		Outcomes: navigationResolutionCounts(stats.outcomes), Confidences: navigationResolutionCounts(stats.confidences), Languages: []NavigationLanguageResolutionStats{},
	}
	if stats.calls > 0 {
		calls := float64(stats.calls)
		result.AmbiguityRate = float64(stats.ambiguous) / calls
		result.ResolutionRate = float64(stats.resolvedLocal) / calls
		result.AmbiguousLocalRate = float64(stats.ambiguousLocal) / calls
		result.UnresolvedLocalRate = float64(stats.unresolvedLocal) / calls
		result.ExpectedExternalRate = float64(stats.expectedExternal) / calls
	}
	return result
}

func navigationResolutionOutcome(call parser.NavigationCall) string {
	switch {
	case call.TargetID != "":
		return "resolved-local"
	case len(call.CandidateTargetIDs) > 1:
		return "ambiguous-local"
	case len(call.CandidateTargetIDs) == 0 && call.ImportPath != "":
		return "expected-external"
	default:
		return "unresolved-local"
	}
}

func navigationLanguageResolutionStats(language string, stats NavigationResolutionStats) NavigationLanguageResolutionStats {
	return NavigationLanguageResolutionStats{
		Language: language, Calls: stats.Calls, Resolved: stats.Resolved, Ambiguous: stats.Ambiguous, Unresolved: stats.Unresolved, Candidate: stats.Candidate, AmbiguityRate: stats.AmbiguityRate,
		ResolvedLocal: stats.ResolvedLocal, AmbiguousLocal: stats.AmbiguousLocal, UnresolvedLocal: stats.UnresolvedLocal, ExpectedExternal: stats.ExpectedExternal,
		ResolutionRate: stats.ResolutionRate, AmbiguousLocalRate: stats.AmbiguousLocalRate, UnresolvedLocalRate: stats.UnresolvedLocalRate, ExpectedExternalRate: stats.ExpectedExternalRate,
		Outcomes: stats.Outcomes, Confidences: stats.Confidences,
	}
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
