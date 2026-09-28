package graph

import (
	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

// Schema identifies the normalized navigation graph projection.
const Schema = analysis.GraphSchema
const navigationGraphSchema = Schema

// Output is the complete normalized navigation graph command projection.
type Output struct {
	Schema           string                             `json:"schema"`
	Files            int                                `json:"files"`
	Metadata         *wire.ResultMetadata               `json:"metadata,omitempty"`
	Sources          SourceSummary                      `json:"sources"`
	Declarations     []parser.NavigationDeclaration     `json:"declarations"`
	TypeDeclarations []parser.NavigationTypeDeclaration `json:"typeDeclarations,omitempty"`
	Imports          []parser.NavigationImport          `json:"imports,omitempty"`
	Calls            []parser.NavigationCall            `json:"calls"`
	Exports          []parser.NavigationExport          `json:"exports,omitempty"`
	Fields           []parser.NavigationField           `json:"fields,omitempty"`
	Resolution       search.NavigationResolutionStats   `json:"resolution"`
	TypeUsages       []parser.NavigationTypeUsage       `json:"typeUsages,omitempty"`
	MemberAccesses   []parser.NavigationMemberAccess    `json:"memberAccesses,omitempty"`
	RepositoryRoots  []string                           `json:"repositoryRoots,omitempty"`
	Query            *Query                             `json:"query,omitempty"`
	Truncation       *Truncation                        `json:"truncation,omitempty"`
}

// Query describes a graph traversal projection.
type Query = analysis.GraphQuery

// Truncation describes source-universe truncation.
type Truncation = analysis.Truncation

// SourceSummary describes graph source processing.
type SourceSummary = analysis.SourceSummary

// BuildOutput constructs a complete graph projection from already resolved paths.
func BuildOutput(paths []string, maxFiles int, options search.NavigationBuildOptions) Output {
	sources := analysis.ReadSources(paths)
	universe, err := analysis.NewUniverseWithOptions(sources, maxFiles, options)
	if err != nil {
		return Output{}
	}
	defer universe.Close()
	report, err := analysis.BuildGraph(universe, nil)
	if err != nil {
		return Output{}
	}
	return FromAnalysis(report)
}

// FromAnalysis adapts a canonical analysis report for command metadata and rendering.
func FromAnalysis(report analysis.GraphReport) Output {
	return Output{
		Schema: report.Schema, Files: report.Files, Sources: report.Sources,
		Declarations: report.Declarations, TypeDeclarations: report.TypeDeclarations,
		Imports: report.Imports, Calls: report.Calls, Exports: report.Exports, Fields: report.Fields,
		Resolution: report.Resolution, TypeUsages: report.TypeUsages, MemberAccesses: report.MemberAccesses,
		RepositoryRoots: report.RepositoryRoots, Query: report.Query, Truncation: report.Truncation,
	}
}
