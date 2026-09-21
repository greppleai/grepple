package cli

import (
	architecturecommand "github.com/greppleai/grepple/internal/cli/architecture"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type directoryArchitecture = architecturecommand.Report
type architectureParsedSource = architecturecommand.ParsedSource
type architectureSourceFile = architecturecommand.SourceFile
type architectureSourceSummary = architecturecommand.SourceSummary
type architectureDirectory = architecturecommand.Directory
type architectureCount = architecturecommand.Count
type architectureSymbol = architecturecommand.Symbol
type architectureRelation = architecturecommand.Relation
type architectureRelationEvidence = architecturecommand.RelationEvidence
type architectureResolveOutput = architecturecommand.ResolveOutput
type architectureWhyOutput = architecturecommand.WhyOutput

func architectureDependencies() architecturecommand.Dependencies {
	return architecturecommand.Dependencies{ApplySourceConfig: applyRepositorySourceConfig, Remote: requestAnalysisRemote, ServerDefault: serverDefault, RequestExit: requestExit}
}
func runArchitecture(args []string) error {
	return architecturecommand.Run(args, architectureDependencies())
}
func runArchitectureDirectory(args []string) error {
	return runArchitecture(append([]string{"directory"}, args...))
}
func runArchitectureResponsibilities(args []string) error {
	return runArchitecture(append([]string{"responsibilities"}, args...))
}
func buildDirectoryArchitecture(paths []string, maxFiles int) (directoryArchitecture, error) {
	return architecturecommand.Build(paths, maxFiles, architectureDependencies())
}
func loadArchitectureDocuments(paths []string) ([]architectureParsedSource, search.NavigationSourceStats) {
	return architecturecommand.LoadDocuments(paths)
}
func architectureNavigationDocuments(sources []architectureParsedSource) []search.NavigationDocumentSource {
	return architecturecommand.NavigationDocuments(sources)
}
func buildDirectoryArchitectureFromParts(paths []string, discovered, supported int, truncation *navigationGraphTruncation, sources []architectureParsedSource, parseStats search.NavigationSourceStats, graph parser.NavigationGraph, graphStats search.NavigationSourceStats) directoryArchitecture {
	var architectureTruncation *architecturecommand.Truncation
	if truncation != nil {
		architectureTruncation = &architecturecommand.Truncation{Reason: truncation.Reason, Limit: truncation.Limit, Skipped: truncation.Skipped}
	}
	return architecturecommand.BuildFromParts(paths, discovered, supported, architectureTruncation, sources, parseStats, graph, graphStats)
}

func resolveArchitectureSymbols(symbols []architectureSymbol, symbol string) []architectureSymbol {
	return architecturecommand.ResolveSymbols(symbols, symbol)
}
func architectureRelationEvidenceFor(relations []architectureRelation, from, to string) []architectureRelationEvidence {
	return architecturecommand.RelationEvidenceFor(relations, from, to)
}
func cleanArchitectureDirectory(value string) string {
	return architecturecommand.CleanDirectory(value)
}
func architectureEvidenceRelation(evidence []architectureRelationEvidence) string {
	return architecturecommand.EvidenceRelation(evidence)
}
func buildArchitectureResponsibilitiesOutput(report directoryArchitecture) architecturecommand.ResponsibilitiesOutput {
	return architecturecommand.BuildResponsibilities(report)
}
func compareDirectoryArchitectures(beforePath, afterPath string, beforeBytes, afterBytes []byte, before, after directoryArchitecture) architecturecommand.Comparison {
	return architecturecommand.Compare(beforePath, afterPath, beforeBytes, afterBytes, before, after)
}
func readDirectoryArchitecture(path string) (directoryArchitecture, []byte, error) {
	return architecturecommand.Read(path)
}

const directoryArchitectureSchema = architecturecommand.Schema

func formatArchitectureCounts(counts []architectureCount) string {
	return architecturecommand.FormatCounts(counts)
}
