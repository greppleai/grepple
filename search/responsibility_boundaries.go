package search

import (
	"fmt"
	"sort"

	"github.com/greppleai/grepple/parser"
)

// ResponsibilityBoundaryCandidate describes a workflow implemented by symbols
// from one owner file and repeated across multiple external files.
type ResponsibilityBoundaryCandidate struct {
	OwnerFile               string                   `json:"ownerFile"`
	Language                string                   `json:"language"`
	Consumers               ResponsibilityBreadth    `json:"consumers"`
	ConsumerDetails         []ResponsibilityConsumer `json:"consumerDetails"`
	ExternalCallableSurface ResponsibilitySurface    `json:"externalCallableSurface"`
	Callables               []string                 `json:"callables"`
	CallableCoUsage         []ResponsibilityPattern  `json:"callableCoUsage"`
	OrderedSequences        []ResponsibilityPattern  `json:"orderedSequences"`
	MemberCallCombinations  []ResponsibilityPattern  `json:"memberCallCombinations"`
	UnresolvedCalls         int                      `json:"unresolvedCalls"`
}

type responsibilityOwnerKey struct {
	language string
	path     string
}

type responsibilityOwnerAnalysis struct {
	callers         map[string]parser.NavigationDeclaration
	callsByCaller   map[string][]string
	interactions    map[string][]responsibilityInteraction
	called          map[string]bool
	declared        int
	unresolvedCalls int
}

// AnalyzeDirectoryResponsibilities uses files as ownership boundaries. It finds
// groups of callables owned by one file whose co-usage or ordering is repeated
// across at least two other files. It does not depend on package/module semantics.
func AnalyzeDirectoryResponsibilities(graph parser.NavigationGraph, minOccurrences int) ([]ResponsibilityBoundaryCandidate, error) {
	if minOccurrences < 1 {
		return nil, fmt.Errorf("minimum occurrences must be positive")
	}
	declarations := indexResponsibilityDeclarations(graph.Declarations)
	owners := indexResponsibilityOwners(graph, declarations)
	candidates := make([]ResponsibilityBoundaryCandidate, 0, len(owners))
	for key, analysis := range owners {
		candidate := buildResponsibilityBoundaryCandidate(key, analysis, minOccurrences)
		if responsibilityBoundaryHasPatterns(candidate) {
			candidates = append(candidates, candidate)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := responsibilityFileBoundaryScore(candidates[i]), responsibilityFileBoundaryScore(candidates[j])
		if left != right {
			return left > right
		}
		if candidates[i].OwnerFile != candidates[j].OwnerFile {
			return candidates[i].OwnerFile < candidates[j].OwnerFile
		}
		return candidates[i].Language < candidates[j].Language
	})
	return candidates, nil
}

func indexResponsibilityDeclarations(declarations []parser.NavigationDeclaration) map[string]parser.NavigationDeclaration {
	indexed := make(map[string]parser.NavigationDeclaration, len(declarations))
	for _, declaration := range declarations {
		indexed[declaration.ID] = declaration
	}
	return indexed
}

func indexResponsibilityOwners(graph parser.NavigationGraph, declarations map[string]parser.NavigationDeclaration) map[responsibilityOwnerKey]*responsibilityOwnerAnalysis {
	owners := make(map[responsibilityOwnerKey]*responsibilityOwnerAnalysis)
	for _, declaration := range graph.Declarations {
		key := responsibilityOwnerKey{language: declaration.Language, path: declaration.Path}
		owner := responsibilityOwner(owners, key)
		owner.declared++
	}
	for _, call := range graph.Calls {
		target, unresolved, ok := responsibilityBoundaryCallTarget(call, declarations)
		caller, callerOK := declarations[call.CallerID]
		if !ok || !callerOK || target.Path == "" || caller.Path == target.Path {
			continue
		}
		key := responsibilityOwnerKey{language: target.Language, path: target.Path}
		owner := responsibilityOwner(owners, key)
		name := responsibilityMemberName(target.Name)
		owner.callers[caller.ID] = caller
		owner.callsByCaller[caller.ID] = append(owner.callsByCaller[caller.ID], name)
		owner.interactions[caller.ID] = append(owner.interactions[caller.ID], responsibilityInteraction{label: name + "()"})
		owner.called[name] = true
		if unresolved {
			owner.unresolvedCalls++
		}
	}
	indexResponsibilityBoundaryMembers(graph.MemberAccesses, declarations, owners)
	return owners
}

func responsibilityOwner(owners map[responsibilityOwnerKey]*responsibilityOwnerAnalysis, key responsibilityOwnerKey) *responsibilityOwnerAnalysis {
	owner := owners[key]
	if owner == nil {
		owner = &responsibilityOwnerAnalysis{callers: make(map[string]parser.NavigationDeclaration), callsByCaller: make(map[string][]string), interactions: make(map[string][]responsibilityInteraction), called: make(map[string]bool)}
		owners[key] = owner
	}
	return owner
}

func responsibilityBoundaryCallTarget(call parser.NavigationCall, declarations map[string]parser.NavigationDeclaration) (parser.NavigationDeclaration, bool, bool) {
	if target, ok := declarations[call.TargetID]; ok {
		return target, false, true
	}
	var selected parser.NavigationDeclaration
	for _, id := range call.CandidateTargetIDs {
		candidate, ok := declarations[id]
		if !ok {
			continue
		}
		if selected.ID == "" {
			selected = candidate
			continue
		}
		if selected.Path != candidate.Path || selected.Language != candidate.Language || responsibilityMemberName(selected.Name) != responsibilityMemberName(candidate.Name) {
			return parser.NavigationDeclaration{}, false, false
		}
	}
	return selected, selected.ID != "", selected.ID != ""
}

func indexResponsibilityBoundaryMembers(accesses []parser.NavigationMemberAccess, declarations map[string]parser.NavigationDeclaration, owners map[responsibilityOwnerKey]*responsibilityOwnerAnalysis) {
	typeOwners := responsibilityTypeOwners(declarations)
	for _, access := range accesses {
		caller, ok := declarations[access.CallerID]
		key, owned := typeOwners[responsibilityTypeCandidate{typeName: terminalTypeName(access.ReceiverType), language: access.Language}]
		if !ok || !owned || caller.Path == key.path {
			continue
		}
		owner := responsibilityOwner(owners, key)
		owner.callers[caller.ID] = caller
		owner.interactions[caller.ID] = append(owner.interactions[caller.ID], responsibilityInteraction{label: access.Member + "(" + access.Operation + ")"})
	}
}

func responsibilityTypeOwners(declarations map[string]parser.NavigationDeclaration) map[responsibilityTypeCandidate]responsibilityOwnerKey {
	owners := make(map[responsibilityTypeCandidate]responsibilityOwnerKey)
	ambiguous := make(map[responsibilityTypeCandidate]bool)
	for _, declaration := range declarations {
		typeName := responsibilityDeclarationType(declaration)
		if typeName == "" || declaration.Path == "" {
			continue
		}
		candidate := responsibilityTypeCandidate{typeName: typeName, language: declaration.Language}
		key := responsibilityOwnerKey{language: declaration.Language, path: declaration.Path}
		if previous, exists := owners[candidate]; exists && previous != key {
			ambiguous[candidate] = true
		}
		owners[candidate] = key
	}
	for candidate := range ambiguous {
		delete(owners, candidate)
	}
	return owners
}

type responsibilityTypeCandidate struct {
	typeName string
	language string
}

func buildResponsibilityBoundaryCandidate(key responsibilityOwnerKey, owner *responsibilityOwnerAnalysis, minimum int) ResponsibilityBoundaryCandidate {
	setGroups := make(map[string]*responsibilityPatternGroup)
	sequenceGroups := make(map[string]*responsibilityPatternGroup)
	combinationGroups := make(map[string]*responsibilityPatternGroup)
	for callerID, caller := range owner.callers {
		consumer := responsibilityConsumer(caller)
		sequence := owner.callsByCaller[callerID]
		if set := uniqueSortedResponsibilityMethods(sequence); len(set) > 1 {
			addResponsibilityPattern(setGroups, set, consumer)
		}
		if len(sequence) > 1 {
			addResponsibilityPattern(sequenceGroups, sequence, consumer)
		}
		combination, hasMember, hasCall := responsibilityCombination(owner.interactions[callerID])
		if hasMember && hasCall {
			addResponsibilityPattern(combinationGroups, combination, consumer)
		}
	}
	candidate := ResponsibilityBoundaryCandidate{
		OwnerFile: key.path, Language: key.language, ExternalCallableSurface: ResponsibilitySurface{External: len(owner.called), Declared: owner.declared},
		Callables: sortedResponsibilityKeys(owner.called), CallableCoUsage: responsibilityCrossFilePatterns(setGroups, minimum),
		OrderedSequences: responsibilityCrossFilePatterns(sequenceGroups, minimum), MemberCallCombinations: responsibilityCrossFilePatterns(combinationGroups, minimum), UnresolvedCalls: owner.unresolvedCalls,
	}
	candidate.ConsumerDetails = responsibilityBoundaryConsumers(candidate)
	candidate.Consumers = responsibilityBreadth(candidate.ConsumerDetails)
	return candidate
}

func responsibilityCrossFilePatterns(groups map[string]*responsibilityPatternGroup, minimum int) []ResponsibilityPattern {
	patterns := responsibilityPatterns(groups, minimum)
	filtered := patterns[:0]
	for _, pattern := range patterns {
		if pattern.Files >= 2 {
			filtered = append(filtered, pattern)
		}
	}
	return filtered
}

func responsibilityBoundaryConsumers(candidate ResponsibilityBoundaryCandidate) []ResponsibilityConsumer {
	indexed := make(map[string]ResponsibilityConsumer)
	for _, patterns := range [][]ResponsibilityPattern{candidate.CallableCoUsage, candidate.OrderedSequences, candidate.MemberCallCombinations} {
		for _, pattern := range patterns {
			for _, consumer := range pattern.Consumers {
				indexed[consumer.ID] = consumer
			}
		}
	}
	consumers := make([]ResponsibilityConsumer, 0, len(indexed))
	for _, consumer := range indexed {
		consumers = append(consumers, consumer)
	}
	sortResponsibilityConsumers(consumers)
	return consumers
}

func responsibilityBoundaryHasPatterns(candidate ResponsibilityBoundaryCandidate) bool {
	return len(candidate.CallableCoUsage) > 0 || len(candidate.OrderedSequences) > 0 || len(candidate.MemberCallCombinations) > 0
}

func responsibilityFileBoundaryScore(candidate ResponsibilityBoundaryCandidate) int {
	score := 0
	for _, patterns := range [][]ResponsibilityPattern{candidate.CallableCoUsage, candidate.OrderedSequences, candidate.MemberCallCombinations} {
		for _, pattern := range patterns {
			score += pattern.Files*1000 + pattern.Occurrences
		}
	}
	return score
}
