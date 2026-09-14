package search

import (
	"fmt"
	"sort"

	"github.com/greppleai/grepple/parser"
)

// BoundaryCandidate describes a workflow implemented by symbols from one owner
// file and repeated across multiple external files.
type BoundaryCandidate struct {
	OwnerFile               string             `json:"ownerFile"`
	Language                string             `json:"language"`
	Risk                    BoundaryRisk       `json:"risk"`
	Reasons                 []string           `json:"reasons"`
	Consumers               BoundaryBreadth    `json:"consumers"`
	ConsumerDetails         []BoundaryConsumer `json:"consumerDetails"`
	ExternalCallableSurface BoundarySurface    `json:"externalCallableSurface"`
	Callables               []string           `json:"callables"`
	CallableCoUsage         []BoundaryPattern  `json:"callableCoUsage"`
	OrderedSequences        []BoundaryPattern  `json:"orderedSequences"`
	MemberCallCombinations  []BoundaryPattern  `json:"memberCallCombinations"`
	UnresolvedCalls         int                `json:"unresolvedCalls"`
}

type boundaryOwnerKey struct {
	language string
	path     string
}

type boundaryOwnerAnalysis struct {
	callers         map[string]parser.NavigationDeclaration
	callsByCaller   map[string][]string
	interactions    map[string][]boundaryInteraction
	called          map[string]bool
	declared        int
	unresolvedCalls int
}

// AnalyzeBoundaries uses files as ownership boundaries. It finds groups of
// callables owned by one file whose co-usage or ordering is repeated across at
// least two other files. It does not depend on package/module semantics.
func AnalyzeBoundaries(graph parser.NavigationGraph, minOccurrences int) ([]BoundaryCandidate, error) {
	if minOccurrences < 1 {
		return nil, fmt.Errorf("minimum occurrences must be positive")
	}
	declarations := indexBoundaryDeclarations(graph.Declarations)
	owners := indexBoundaryOwners(graph, declarations)
	candidates := make([]BoundaryCandidate, 0, len(owners))
	for key, analysis := range owners {
		candidate := buildBoundaryCandidate(key, analysis, minOccurrences)
		if boundaryHasPatterns(candidate) {
			candidates = append(candidates, candidate)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftRisk, rightRisk := boundaryRiskRank(candidates[i].Risk), boundaryRiskRank(candidates[j].Risk)
		if leftRisk != rightRisk {
			return leftRisk > rightRisk
		}
		left, right := boundaryScore(candidates[i]), boundaryScore(candidates[j])
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

func indexBoundaryDeclarations(declarations []parser.NavigationDeclaration) map[string]parser.NavigationDeclaration {
	indexed := make(map[string]parser.NavigationDeclaration, len(declarations))
	for _, declaration := range declarations {
		indexed[declaration.ID] = declaration
	}
	return indexed
}

func indexBoundaryOwners(graph parser.NavigationGraph, declarations map[string]parser.NavigationDeclaration) map[boundaryOwnerKey]*boundaryOwnerAnalysis {
	owners := make(map[boundaryOwnerKey]*boundaryOwnerAnalysis)
	for _, declaration := range graph.Declarations {
		owner := getBoundaryOwner(owners, boundaryOwnerKey{language: declaration.Language, path: declaration.Path})
		owner.declared++
	}
	for _, call := range graph.Calls {
		target, unresolved, ok := boundaryCallTarget(call, declarations)
		caller, callerOK := declarations[call.CallerID]
		if !ok || !callerOK || target.Path == "" || caller.Path == target.Path {
			continue
		}
		owner := getBoundaryOwner(owners, boundaryOwnerKey{language: target.Language, path: target.Path})
		name := boundaryCallableName(target.Name)
		owner.callers[caller.ID] = caller
		owner.callsByCaller[caller.ID] = append(owner.callsByCaller[caller.ID], name)
		owner.interactions[caller.ID] = append(owner.interactions[caller.ID], boundaryInteraction{label: name + "()"})
		owner.called[name] = true
		if unresolved {
			owner.unresolvedCalls++
		}
	}
	indexBoundaryMembers(graph.MemberAccesses, declarations, owners)
	return owners
}

func getBoundaryOwner(owners map[boundaryOwnerKey]*boundaryOwnerAnalysis, key boundaryOwnerKey) *boundaryOwnerAnalysis {
	owner := owners[key]
	if owner == nil {
		owner = &boundaryOwnerAnalysis{callers: make(map[string]parser.NavigationDeclaration), callsByCaller: make(map[string][]string), interactions: make(map[string][]boundaryInteraction), called: make(map[string]bool)}
		owners[key] = owner
	}
	return owner
}

func boundaryCallTarget(call parser.NavigationCall, declarations map[string]parser.NavigationDeclaration) (parser.NavigationDeclaration, bool, bool) {
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
		if selected.Path != candidate.Path || selected.Language != candidate.Language || boundaryCallableName(selected.Name) != boundaryCallableName(candidate.Name) {
			return parser.NavigationDeclaration{}, false, false
		}
	}
	return selected, selected.ID != "", selected.ID != ""
}

func indexBoundaryMembers(accesses []parser.NavigationMemberAccess, declarations map[string]parser.NavigationDeclaration, owners map[boundaryOwnerKey]*boundaryOwnerAnalysis) {
	typeOwners := boundaryTypeOwners(declarations)
	for _, access := range accesses {
		caller, ok := declarations[access.CallerID]
		key, owned := typeOwners[boundaryTypeCandidate{typeName: boundaryTerminalTypeName(access.ReceiverType), language: access.Language}]
		if !ok || !owned || caller.Path == key.path {
			continue
		}
		owner := getBoundaryOwner(owners, key)
		owner.callers[caller.ID] = caller
		owner.interactions[caller.ID] = append(owner.interactions[caller.ID], boundaryInteraction{label: access.Member + "(" + access.Operation + ")"})
	}
}

func boundaryTypeOwners(declarations map[string]parser.NavigationDeclaration) map[boundaryTypeCandidate]boundaryOwnerKey {
	owners := make(map[boundaryTypeCandidate]boundaryOwnerKey)
	ambiguous := make(map[boundaryTypeCandidate]bool)
	for _, declaration := range declarations {
		typeName := boundaryDeclarationType(declaration)
		if typeName == "" || declaration.Path == "" {
			continue
		}
		candidate := boundaryTypeCandidate{typeName: typeName, language: declaration.Language}
		key := boundaryOwnerKey{language: declaration.Language, path: declaration.Path}
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

type boundaryTypeCandidate struct {
	typeName string
	language string
}

func buildBoundaryCandidate(key boundaryOwnerKey, owner *boundaryOwnerAnalysis, minimum int) BoundaryCandidate {
	setGroups := make(map[string]*boundaryPatternGroup)
	sequenceGroups := make(map[string]*boundaryPatternGroup)
	combinationGroups := make(map[string]*boundaryPatternGroup)
	for callerID, caller := range owner.callers {
		consumer := boundaryConsumer(caller)
		sequence := owner.callsByCaller[callerID]
		if set := uniqueSortedBoundaryInteractions(sequence); len(set) > 1 {
			addBoundaryPattern(setGroups, set, consumer)
		}
		if len(sequence) > 1 {
			addBoundaryPattern(sequenceGroups, sequence, consumer)
		}
		combination, hasMember, hasCall := boundaryCombination(owner.interactions[callerID])
		if hasMember && hasCall {
			addBoundaryPattern(combinationGroups, combination, consumer)
		}
	}
	candidate := BoundaryCandidate{
		OwnerFile: key.path, Language: key.language, ExternalCallableSurface: BoundarySurface{External: len(owner.called), Declared: owner.declared},
		Callables: sortedBoundaryKeys(owner.called), CallableCoUsage: crossFileBoundaryPatterns(setGroups, minimum),
		OrderedSequences: crossFileBoundaryPatterns(sequenceGroups, minimum), MemberCallCombinations: crossFileBoundaryPatterns(combinationGroups, minimum), UnresolvedCalls: owner.unresolvedCalls,
	}
	candidate.ConsumerDetails = boundaryConsumers(candidate)
	candidate.Consumers = boundaryBreadth(candidate.ConsumerDetails)
	candidate.Risk, candidate.Reasons = classifyWorkflowBoundary(candidate)
	return candidate
}

func crossFileBoundaryPatterns(groups map[string]*boundaryPatternGroup, minimum int) []BoundaryPattern {
	patterns := boundaryPatterns(groups, minimum)
	filtered := patterns[:0]
	for _, pattern := range patterns {
		if pattern.Files >= 2 {
			filtered = append(filtered, pattern)
		}
	}
	return filtered
}

func boundaryConsumers(candidate BoundaryCandidate) []BoundaryConsumer {
	indexed := make(map[string]BoundaryConsumer)
	for _, patterns := range [][]BoundaryPattern{candidate.CallableCoUsage, candidate.OrderedSequences, candidate.MemberCallCombinations} {
		for _, pattern := range patterns {
			for _, consumer := range pattern.Consumers {
				indexed[consumer.ID] = consumer
			}
		}
	}
	consumers := make([]BoundaryConsumer, 0, len(indexed))
	for _, consumer := range indexed {
		consumers = append(consumers, consumer)
	}
	sortBoundaryConsumers(consumers)
	return consumers
}

func boundaryHasPatterns(candidate BoundaryCandidate) bool {
	return len(candidate.CallableCoUsage) > 0 || len(candidate.OrderedSequences) > 0 || len(candidate.MemberCallCombinations) > 0
}

func classifyWorkflowBoundary(candidate BoundaryCandidate) (BoundaryRisk, []string) {
	risk := BoundaryRiskLow
	reasons := []string{"repeated-owner-file-workflow"}
	if candidate.Consumers.Packages > 1 {
		risk = BoundaryRiskMedium
		reasons = append(reasons, "cross-package-workflow")
	}
	if candidate.ExternalCallableSurface.Declared > 0 && candidate.ExternalCallableSurface.External*2 >= candidate.ExternalCallableSurface.Declared {
		reasons = append(reasons, "broad-owner-callable-surface")
	}
	if candidate.UnresolvedCalls > 0 {
		reasons = append(reasons, "candidate-resolved-interactions")
	}
	return risk, reasons
}

func boundaryScore(candidate BoundaryCandidate) int {
	score := 0
	for _, patterns := range [][]BoundaryPattern{candidate.CallableCoUsage, candidate.OrderedSequences, candidate.MemberCallCombinations} {
		for _, pattern := range patterns {
			score += pattern.Files*1000 + pattern.Occurrences
		}
	}
	return score
}
