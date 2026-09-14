package search

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// BoundaryPolicySchema identifies the supported repository policy contract.
const BoundaryPolicySchema = "grepple-boundary-policy-v1"

// BoundaryPolicy is repository-owned evidence for layers, containment, facades,
// and intentional path roles. Empty policy leaves non-structural classifications unknown.
type BoundaryPolicy struct {
	Schema          string                       `json:"schema"`
	Layers          []BoundaryLayer              `json:"layers,omitempty"`
	Containments    []BoundaryContainmentRule    `json:"containments,omitempty"`
	Facades         []BoundaryFacadeRule         `json:"facades,omitempty"`
	Classifications []BoundaryPathClassification `json:"classifications,omitempty"`
}

// BoundaryLayer maps repository paths to one architecture layer.
type BoundaryLayer struct {
	Name  string   `json:"name"`
	Paths []string `json:"paths"`
}

// BoundaryContainmentRule declares consumer paths approved for owned paths.
type BoundaryContainmentRule struct {
	Name          string   `json:"name"`
	OwnerPaths    []string `json:"ownerPaths"`
	ConsumerPaths []string `json:"consumerPaths"`
}

// BoundaryFacadeRule declares facade and implementation path sets.
type BoundaryFacadeRule struct {
	Name                string   `json:"name"`
	FacadePaths         []string `json:"facadePaths"`
	ImplementationPaths []string `json:"implementationPaths"`
}

// BoundaryPathClassification marks paths with an intentional repository role.
type BoundaryPathClassification struct {
	Category string   `json:"category"`
	Paths    []string `json:"paths"`
}

// BoundarySpread classifies the widest evidenced architecture reach.
type BoundarySpread string

// Boundary spread values are ordered by increasing architectural reach.
const (
	BoundarySpreadPackageInternal BoundarySpread = "package-internal"
	BoundarySpreadCrossPackage    BoundarySpread = "cross-package"
	BoundarySpreadCrossLayer      BoundarySpread = "cross-layer"
	BoundarySpreadPublicAPI       BoundarySpread = "public-api"
)

// BoundaryContainment classifies evidence against repository containment policy.
type BoundaryContainment string

// Boundary containment values distinguish evidence from unknown intent.
const (
	BoundaryContainmentApproved BoundaryContainment = "approved"
	BoundaryContainmentEscaped  BoundaryContainment = "escaped"
	BoundaryContainmentUnknown  BoundaryContainment = "unknown"
)

// BoundaryFacadeBypass identifies one direct resolved call around a declared facade.
type BoundaryFacadeBypass struct {
	Facade         string           `json:"facade"`
	Implementation string           `json:"implementation"`
	Caller         BoundaryConsumer `json:"caller"`
	Target         BoundaryConsumer `json:"target"`
	Confidence     string           `json:"confidence"`
}

// ValidateBoundaryPolicy rejects unsupported or incomplete repository policy.
func ValidateBoundaryPolicy(policy BoundaryPolicy) error {
	if policy.Schema != "" && policy.Schema != BoundaryPolicySchema {
		return fmt.Errorf("unsupported boundary policy schema %q", policy.Schema)
	}
	if err := validateBoundaryLayers(policy.Layers); err != nil {
		return err
	}
	if err := validateBoundaryContainments(policy.Containments); err != nil {
		return err
	}
	if err := validateBoundaryFacades(policy.Facades); err != nil {
		return err
	}
	return validateBoundaryClassifications(policy.Classifications)
}

func validateBoundaryLayers(layers []BoundaryLayer) error {
	for _, layer := range layers {
		if layer.Name == "" || len(layer.Paths) == 0 {
			return fmt.Errorf("boundary layer requires name and paths")
		}
	}
	return nil
}

func validateBoundaryContainments(rules []BoundaryContainmentRule) error {
	for _, rule := range rules {
		if rule.Name == "" || len(rule.OwnerPaths) == 0 || len(rule.ConsumerPaths) == 0 {
			return fmt.Errorf("boundary containment requires name, ownerPaths, and consumerPaths")
		}
	}
	return nil
}

func validateBoundaryFacades(rules []BoundaryFacadeRule) error {
	for _, rule := range rules {
		if rule.Name == "" || len(rule.FacadePaths) == 0 || len(rule.ImplementationPaths) == 0 {
			return fmt.Errorf("boundary facade requires name, facadePaths, and implementationPaths")
		}
	}
	return nil
}

func validateBoundaryClassifications(classifications []BoundaryPathClassification) error {
	for _, classification := range classifications {
		category := normalizedBoundaryCategory(classification.Category)
		if !supportedBoundaryPathCategory(category) || len(classification.Paths) == 0 {
			return fmt.Errorf("unsupported or empty boundary path classification %q", classification.Category)
		}
	}
	return nil
}

func supportedBoundaryPathCategory(category string) bool {
	switch category {
	case "utility-hub", "test-framework", "declarative-configuration", "lifecycle-cleanup", "adapter-protocol":
		return true
	default:
		return false
	}
}

func boundaryPathMatches(patterns []string, value string) bool {
	value = strings.TrimPrefix(path.Clean(strings.ReplaceAll(value, "\\", "/")), "./")
	for _, pattern := range patterns {
		pattern = strings.TrimPrefix(path.Clean(strings.ReplaceAll(pattern, "\\", "/")), "./")
		if strings.ContainsAny(pattern, "*?[") {
			if matchSegments(strings.Split(pattern, "/"), strings.Split(value, "/")) {
				return true
			}
			continue
		}
		if value == pattern || strings.HasPrefix(value, strings.TrimSuffix(pattern, "/")+"/") {
			return true
		}
	}
	return false
}

func boundaryLayerForPath(policy BoundaryPolicy, value string) string {
	for _, layer := range policy.Layers {
		if boundaryPathMatches(layer.Paths, value) {
			return layer.Name
		}
	}
	return ""
}

func boundaryPathCategories(policy BoundaryPolicy, value string) []string {
	set := map[string]bool{}
	for _, classification := range policy.Classifications {
		if boundaryPathMatches(classification.Paths, value) {
			set[normalizedBoundaryCategory(classification.Category)] = true
		}
	}
	result := make([]string, 0, len(set))
	for category := range set {
		result = append(result, category)
	}
	sort.Strings(result)
	return result
}

func boundaryContainmentForConsumers(policy BoundaryPolicy, owner string, consumers []BoundaryConsumer) BoundaryContainment {
	for _, rule := range policy.Containments {
		if !boundaryPathMatches(rule.OwnerPaths, owner) {
			continue
		}
		for _, consumer := range consumers {
			if !boundaryPathMatches(rule.ConsumerPaths, consumer.Path) {
				return BoundaryContainmentEscaped
			}
		}
		return BoundaryContainmentApproved
	}
	if len(consumers) > 0 {
		ownerPackage := path.Dir(strings.ReplaceAll(owner, "\\", "/"))
		for _, consumer := range consumers {
			if path.Dir(strings.ReplaceAll(consumer.Path, "\\", "/")) != ownerPackage {
				return BoundaryContainmentUnknown
			}
		}
		return BoundaryContainmentApproved
	}
	return BoundaryContainmentUnknown
}

// AnalyzeFacadeBypasses reports resolved direct calls into policy-declared
// implementation paths from outside both the implementation and its facade.
func AnalyzeFacadeBypasses(graph parser.NavigationGraph, policy BoundaryPolicy) []BoundaryFacadeBypass {
	declarations := indexBoundaryDeclarations(graph.Declarations)
	bypasses := []BoundaryFacadeBypass{}
	seen := map[string]bool{}
	for _, call := range graph.Calls {
		for _, bypass := range boundaryFacadeBypassesForCall(call, declarations, policy.Facades) {
			key := strings.Join([]string{bypass.Facade, call.CallerID, call.TargetID, fmt.Sprint(call.Line)}, "\x00")
			if !seen[key] {
				seen[key] = true
				bypasses = append(bypasses, bypass)
			}
		}
	}
	sort.Slice(bypasses, func(i, j int) bool {
		if bypasses[i].Facade != bypasses[j].Facade {
			return bypasses[i].Facade < bypasses[j].Facade
		}
		if bypasses[i].Caller.Path != bypasses[j].Caller.Path {
			return bypasses[i].Caller.Path < bypasses[j].Caller.Path
		}
		if bypasses[i].Caller.Line != bypasses[j].Caller.Line {
			return bypasses[i].Caller.Line < bypasses[j].Caller.Line
		}
		return bypasses[i].Target.ID < bypasses[j].Target.ID
	})
	return bypasses
}

func boundaryFacadeBypassesForCall(call parser.NavigationCall, declarations map[string]parser.NavigationDeclaration, rules []BoundaryFacadeRule) []BoundaryFacadeBypass {
	target, targetOK := declarations[call.TargetID]
	caller, callerOK := declarations[call.CallerID]
	if !targetOK || !callerOK {
		return nil
	}
	result := []BoundaryFacadeBypass{}
	for _, rule := range rules {
		if !boundaryPathMatches(rule.ImplementationPaths, target.Path) || boundaryPathMatches(rule.ImplementationPaths, caller.Path) || boundaryPathMatches(rule.FacadePaths, caller.Path) {
			continue
		}
		callerEvidence := boundaryConsumer(caller)
		if call.Line > 0 {
			callerEvidence.Line = call.Line
		}
		result = append(result, BoundaryFacadeBypass{Facade: rule.Name, Implementation: target.Path, Caller: callerEvidence, Target: boundaryConsumer(target), Confidence: call.Confidence})
	}
	return result
}
