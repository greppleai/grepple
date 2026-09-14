package search

import (
	"path"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

func indexBoundaryFields(fields []parser.NavigationField, _ map[string]parser.NavigationDeclaration, owners map[boundaryOwnerKey]*boundaryOwnerAnalysis) {
	declared := map[boundaryOwnerKey]map[string]bool{}
	for _, field := range fields {
		key := boundaryOwnerKey{language: field.Language, path: field.Path}
		if declared[key] == nil {
			declared[key] = map[string]bool{}
		}
		declared[key][field.Name] = true
	}
	for key, owner := range owners {
		names := declared[key]
		owner.declaredFields = len(names)
		for accessed := range owner.accessedFields {
			if !names[accessed] {
				delete(owner.accessedFields, accessed)
			}
		}
	}
}

func classifyWorkflowCandidatePolicy(candidate *BoundaryCandidate, policy BoundaryPolicy) {
	candidate.Spread = boundaryWorkflowSpread(*candidate, policy)
	candidate.Containment = boundaryContainmentForConsumers(policy, candidate.OwnerFile, candidate.ConsumerDetails)
	if len(candidate.OrderedSequences) > 0 {
		candidate.Signals = appendUniqueBoundaryReason(candidate.Signals, "repeated-protocol")
	}
	if candidate.ExternalCallableSurface.Declared > 0 && candidate.ExternalCallableSurface.External*2 >= candidate.ExternalCallableSurface.Declared && candidate.Spread == BoundarySpreadPackageInternal {
		candidate.Signals = appendUniqueBoundaryReason(candidate.Signals, "owner-cohesion")
	}
	categories := boundaryPathCategories(policy, candidate.OwnerFile)
	for _, category := range categories {
		switch category {
		case "utility-hub":
			candidate.Signals = appendUniqueBoundaryReason(candidate.Signals, "utility-hub")
			candidate.Risk = downrankBoundaryRisk(candidate.Risk)
		case "declarative-configuration", "lifecycle-cleanup", "adapter-protocol":
			candidate.Signals = appendUniqueBoundaryReason(candidate.Signals, category)
			candidate.Risk = downrankBoundaryRisk(candidate.Risk)
		}
	}
	if candidate.Containment == BoundaryContainmentEscaped {
		candidate.Reasons = appendUniqueBoundaryReason(candidate.Reasons, "containment-escaped")
		if candidate.ExternalCallableSurface.Declared == 1 && candidate.ExternalCallableSurface.External == 1 {
			candidate.Signals = appendUniqueBoundaryReason(candidate.Signals, "misplaced-function")
		}
	}
	sort.Strings(candidate.Signals)
}

func boundaryWorkflowSpread(candidate BoundaryCandidate, policy BoundaryPolicy) BoundarySpread {
	layers := map[string]bool{}
	if layer := boundaryLayerForPath(policy, candidate.OwnerFile); layer != "" {
		layers[layer] = true
	}
	packages := map[string]bool{path.Dir(strings.ReplaceAll(candidate.OwnerFile, "\\", "/")): true}
	for _, consumer := range candidate.ConsumerDetails {
		packages[path.Dir(strings.ReplaceAll(consumer.Path, "\\", "/"))] = true
		if layer := boundaryLayerForPath(policy, consumer.Path); layer != "" {
			layers[layer] = true
		}
	}
	if len(layers) > 1 {
		return BoundarySpreadCrossLayer
	}
	if len(packages) > 1 {
		return BoundarySpreadCrossPackage
	}
	return BoundarySpreadPackageInternal
}

func classifyParallelBoundaryAbstractions(candidates []BoundaryCandidate) {
	groups := map[string][]int{}
	for index, candidate := range candidates {
		if len(candidate.Callables) < 2 {
			continue
		}
		key := candidate.Language + "\x00" + strings.Join(candidate.Callables, "\x00")
		groups[key] = append(groups[key], index)
	}
	for _, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			candidates[index].Signals = appendUniqueBoundaryReason(candidates[index].Signals, "parallel-abstraction")
			sort.Strings(candidates[index].Signals)
		}
	}
}
