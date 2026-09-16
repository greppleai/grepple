package analysis

import (
	"sort"

	"github.com/greppleai/grepple/search"
)

// BoundaryReport is the complete graph-backed boundary analysis.
type BoundaryReport struct {
	Schema         string                        `json:"schema"`
	Paths          []string                      `json:"paths"`
	Files          int                           `json:"files"`
	Sources        SourceSummary                 `json:"sources"`
	Policy         string                        `json:"policy,omitempty"`
	Candidates     []search.BoundaryCandidate    `json:"candidates"`
	TypeBoundaries []search.BoundaryTypeSpread   `json:"typeBoundaries"`
	FacadeBypasses []search.BoundaryFacadeBypass `json:"facadeBypasses,omitempty"`
	Truncation     *Truncation                   `json:"truncation,omitempty"`
}

// BuildBoundaries applies validated repository-owned policy to one universe.
func BuildBoundaries(universe *Universe, paths []string, minimum int, policy search.BoundaryPolicy, policyPath string) (BoundaryReport, error) {
	if minimum < 1 {
		minimum = 2
	}
	if err := search.ValidateBoundaryPolicy(policy); err != nil {
		return BoundaryReport{}, err
	}
	candidates, err := search.AnalyzeBoundariesWithPolicy(universe.graph, minimum, policy)
	if err != nil {
		return BoundaryReport{}, err
	}
	types, err := search.AnalyzeTypeBoundariesWithPolicy(universe.graph, minimum, policy)
	if err != nil {
		return BoundaryReport{}, err
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return BoundaryReport{Schema: "grepple-boundaries-v3", Paths: paths, Files: len(universe.paths), Sources: universe.Summary(), Policy: policyPath, Candidates: candidates, TypeBoundaries: types, FacadeBypasses: search.AnalyzeFacadeBypasses(universe.graph, policy), Truncation: universe.Truncation()}, nil
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
	Routes          int                 `json:"routes"`
	Incoming        int                 `json:"incomingRelations"`
	Outgoing        int                 `json:"outgoingRelations"`
}

// BuildResponsibilities derives a deterministic ownership summary from directory architecture.
func BuildResponsibilities(universe *Universe) ResponsibilityReport {
	architecture := BuildArchitecture(universe)
	incoming, outgoing := map[string]int{}, map[string]int{}
	for _, relation := range architecture.Relations {
		outgoing[relation.From] += relation.Count
		incoming[relation.To] += relation.Count
	}
	items := make([]DirectoryResponsibility, 0, len(architecture.Directories))
	for _, directory := range architecture.Directories {
		items = append(items, DirectoryResponsibility{Directory: directory.Path, Files: directory.Files, Classifications: directory.Classifications, Languages: directory.Languages, Declarations: directory.Declarations, PublicCallables: directory.PublicCallables, Entrypoints: directory.Entrypoints, Routes: directory.Routes, Incoming: incoming[directory.Path], Outgoing: outgoing[directory.Path]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Directory < items[j].Directory })
	return ResponsibilityReport{Schema: "grepple-directory-responsibilities-v1", Files: architecture.Files, Sources: architecture.Sources, Responsibilities: items, Truncation: architecture.Truncation}
}
