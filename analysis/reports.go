package analysis

import (
	"sort"

	boundaryanalysis "github.com/greppleai/grepple/internal/boundaryanalysis"
	"github.com/greppleai/grepple/parser"
)

type BoundaryPolicy = boundaryanalysis.BoundaryPolicy
type BoundaryLayer = boundaryanalysis.BoundaryLayer
type BoundaryContainmentRule = boundaryanalysis.BoundaryContainmentRule
type BoundaryFacadeRule = boundaryanalysis.BoundaryFacadeRule
type BoundaryPathClassification = boundaryanalysis.BoundaryPathClassification
type BoundaryCandidate = boundaryanalysis.BoundaryCandidate
type BoundaryTypeSpread = boundaryanalysis.BoundaryTypeSpread
type BoundaryFacadeBypass = boundaryanalysis.BoundaryFacadeBypass
type BoundaryRisk = boundaryanalysis.BoundaryRisk
type BoundaryConsumer = boundaryanalysis.BoundaryConsumer
type BoundaryPattern = boundaryanalysis.BoundaryPattern
type BoundaryTypeOrigin = boundaryanalysis.BoundaryTypeOrigin
type BoundaryTypeUsage = boundaryanalysis.BoundaryTypeUsage

const BoundaryPolicySchema = boundaryanalysis.BoundaryPolicySchema

const (
	BoundaryTypeOriginLocal           = boundaryanalysis.BoundaryTypeOriginLocal
	BoundaryTypeOriginFirstParty      = boundaryanalysis.BoundaryTypeOriginFirstParty
	BoundaryTypeOriginStandardLibrary = boundaryanalysis.BoundaryTypeOriginStandardLibrary
	BoundaryTypeOriginThirdParty      = boundaryanalysis.BoundaryTypeOriginThirdParty
	BoundaryTypeOriginUnresolved      = boundaryanalysis.BoundaryTypeOriginUnresolved
)

// BoundaryAnalysis is the reusable graph-backed boundary result.
type BoundaryAnalysis struct {
	Candidates     []BoundaryCandidate
	TypeBoundaries []BoundaryTypeSpread
	FacadeBypasses []BoundaryFacadeBypass
}

// ValidateBoundaryPolicy validates repository-owned boundary policy.
func ValidateBoundaryPolicy(policy BoundaryPolicy) error {
	return boundaryanalysis.ValidateBoundaryPolicy(policy)
}

// AnalyzeBoundaries applies repository policy to an existing navigation graph.
func AnalyzeBoundaries(graph parser.NavigationGraph, minimum int, policy BoundaryPolicy) (BoundaryAnalysis, error) {
	if minimum < 1 {
		minimum = 2
	}
	if err := boundaryanalysis.ValidateBoundaryPolicy(policy); err != nil {
		return BoundaryAnalysis{}, err
	}
	candidates, err := boundaryanalysis.AnalyzeBoundariesWithPolicy(graph, minimum, policy)
	if err != nil {
		return BoundaryAnalysis{}, err
	}
	types, err := boundaryanalysis.AnalyzeTypeBoundariesWithPolicy(graph, minimum, policy)
	if err != nil {
		return BoundaryAnalysis{}, err
	}
	return BoundaryAnalysis{Candidates: candidates, TypeBoundaries: types, FacadeBypasses: boundaryanalysis.AnalyzeFacadeBypasses(graph, policy)}, nil
}

// BoundaryReport is the complete graph-backed boundary analysis.
type BoundaryReport struct {
	Schema         string                 `json:"schema"`
	Paths          []string               `json:"paths"`
	Files          int                    `json:"files"`
	Sources        SourceSummary          `json:"sources"`
	Policy         string                 `json:"policy,omitempty"`
	Candidates     []BoundaryCandidate    `json:"candidates"`
	TypeBoundaries []BoundaryTypeSpread   `json:"typeBoundaries"`
	FacadeBypasses []BoundaryFacadeBypass `json:"facadeBypasses,omitempty"`
	Truncation     *Truncation            `json:"truncation,omitempty"`
}

// BuildBoundaries applies validated repository-owned policy to one universe.
func BuildBoundaries(universe *Universe, paths []string, minimum int, policy BoundaryPolicy, policyPath string) (BoundaryReport, error) {
	result, err := AnalyzeBoundaries(universe.graph, minimum, policy)
	if err != nil {
		return BoundaryReport{}, err
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return BoundaryReport{Schema: "grepple-boundaries-v3", Paths: paths, Files: len(universe.paths), Sources: universe.Summary(), Policy: policyPath, Candidates: result.Candidates, TypeBoundaries: result.TypeBoundaries, FacadeBypasses: result.FacadeBypasses, Truncation: universe.Truncation()}, nil
}

// ResponsibilityReport summarizes what each directory owns and how it participates in relations.
type ResponsibilityReport struct {
	Schema           string                    `json:"schema"`
	Files            int                       `json:"files"`
	Sources          SourceSummary             `json:"sources"`
	Responsibilities []DirectoryResponsibility `json:"responsibilities"`
	Truncation       *Truncation               `json:"truncation,omitempty"`
}

// DirectoryResponsibility summarizes ownership and relation participation for one directory.
type DirectoryResponsibility struct {
	Directory       string              `json:"directory"`
	Files           int                 `json:"files"`
	Classifications []ArchitectureCount `json:"classifications"`
	Languages       []ArchitectureCount `json:"languages"`
	Declarations    []ArchitectureCount `json:"declarations"`
	PublicCallables int                 `json:"publicCallables"`
	Entrypoints     int                 `json:"entrypoints"`
	Incoming        int                 `json:"incomingRelations"`
	Outgoing        int                 `json:"outgoingRelations"`
}

// BuildResponsibilities derives a deterministic ownership summary from directory architecture.
func BuildResponsibilities(universe *Universe) ResponsibilityReport {
	return BuildResponsibilitiesFromArchitecture(BuildArchitecture(universe))
}

// BuildResponsibilitiesFromArchitecture derives responsibilities from an existing architecture report.
func BuildResponsibilitiesFromArchitecture(architecture ArchitectureReport) ResponsibilityReport {
	incoming, outgoing := map[string]int{}, map[string]int{}
	for _, relation := range architecture.Relations {
		outgoing[relation.From] += relation.Count
		incoming[relation.To] += relation.Count
	}
	items := make([]DirectoryResponsibility, 0, len(architecture.Directories))
	for _, directory := range architecture.Directories {
		items = append(items, DirectoryResponsibility{Directory: directory.Path, Files: directory.Files, Classifications: directory.Classifications, Languages: directory.Languages, Declarations: directory.Declarations, PublicCallables: directory.PublicCallables, Entrypoints: directory.Entrypoints, Incoming: incoming[directory.Path], Outgoing: outgoing[directory.Path]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Directory < items[j].Directory })
	return ResponsibilityReport{Schema: "grepple-directory-responsibilities-v2", Files: architecture.Files, Sources: architecture.Sources, Responsibilities: items, Truncation: architecture.Truncation}
}
