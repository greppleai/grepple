package graph

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const navigationGraphSchema = "grepple-navigation-graph-v7"

// Output is the complete normalized navigation graph command projection.
type Output struct {
	Schema           string                             `json:"schema"`
	Files            int                                `json:"files"`
	Metadata         *api.ResultMetadata                `json:"metadata,omitempty"`
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
type Query struct {
	Direction    string   `json:"direction"`
	Depth        int      `json:"depth"`
	RootIDs      []string `json:"rootIds"`
	Languages    []string `json:"languages,omitempty"`
	Confidences  []string `json:"confidences,omitempty"`
	Visibilities []string `json:"visibilities,omitempty"`
}

// Truncation describes source-universe truncation.
type Truncation struct {
	Reason  string `json:"reason"`
	Limit   int    `json:"limit"`
	Skipped int    `json:"skipped"`
}

// SourceSummary describes graph source processing.
type SourceSummary struct {
	Discovered int `json:"discovered"`
	Selected   int `json:"selected"`
	Parsed     int `json:"parsed"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
	Recovered  int `json:"recovered"`
}

// BuildOutput constructs a complete graph projection from already resolved paths.
func BuildOutput(paths []string, maxFiles int, options search.NavigationBuildOptions) Output {
	discovered := len(paths)
	eligible := SourcePaths(paths)
	unsupported := discovered - len(eligible)
	var truncation *Truncation
	if maxFiles > 0 && len(eligible) > maxFiles {
		truncation = &Truncation{Reason: "max_files", Limit: maxFiles, Skipped: len(eligible) - maxFiles}
		eligible = eligible[:maxFiles]
	}
	graph, stats := search.BuildNavigationGraphWithOptions(eligible, options)
	return OutputFromParts(eligible, discovered, unsupported, truncation, graph, stats)
}

// OutputFromParts projects an existing parser graph and source statistics.
func OutputFromParts(paths []string, discovered, unsupported int, truncation *Truncation, graph parser.NavigationGraph, stats search.NavigationSourceStats) Output {
	declarations := graph.Declarations
	if declarations == nil {
		declarations = []parser.NavigationDeclaration{}
	}
	calls := graph.Calls
	if calls == nil {
		calls = []parser.NavigationCall{}
	}
	return Output{
		Schema: navigationGraphSchema, Files: len(paths),
		Sources:      SourceSummary{Discovered: discovered, Selected: stats.Attempted, Parsed: stats.Parsed, Skipped: unsupported + stats.Skipped, Failed: stats.Failed, Recovered: stats.Recovered},
		Declarations: declarations, TypeDeclarations: graph.TypeDeclarations, Calls: calls, Imports: graph.Imports, Exports: graph.Exports, Fields: graph.Fields, TypeUsages: graph.TypeUsages, MemberAccesses: graph.MemberAccesses, RepositoryRoots: graph.RepositoryRoots, Resolution: search.MeasureNavigationResolution(graph), Truncation: truncation,
	}
}

// SourcePaths retains files whose parser adapter supports navigation facts.
func SourcePaths(paths []string) []string {
	sources := make([]string, 0, len(paths))
	for _, path := range paths {
		language := parser.LanguageFor(path)
		capabilities, ok := parser.CapabilitiesForLanguage(language)
		if ok && capabilities.Navigation {
			sources = append(sources, path)
		}
	}
	return sources
}
