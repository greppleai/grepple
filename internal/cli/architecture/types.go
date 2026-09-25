package architecture

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/greppleai/grepple/analysis"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const defaultTextOutputBytes = 16 * 1024

type commonArgs = cliruntime.CommonArgs

// SourceSummary describes source-universe processing.
type SourceSummary = analysis.SourceSummary

// Truncation describes a source-universe limit.
type Truncation = analysis.Truncation

type Report = analysis.ArchitectureReport
type architectureSourceFile = analysis.ArchitectureSourceFile
type architectureDirectory = analysis.ArchitectureDirectory
type architectureCount = analysis.ArchitectureCount
type architectureSymbol = analysis.ArchitectureSymbol
type architectureRelation = analysis.ArchitectureRelation
type architectureRelationEvidence = analysis.ArchitectureRelationEvidence
type architectureRelationCoverage = analysis.ArchitectureRelationCoverage

type directoryArchitecture = Report
type navigationGraphTruncation = Truncation
type navigationSourceSummary = SourceSummary

func isExtractHelp(value string) bool { return value == "--help" || value == "-h" || value == "help" }

// Schema identifies complete directory architecture reports.
const Schema = directoryArchitectureSchema

// SourceFile is one classified source in an architecture report.
type SourceFile = architectureSourceFile

// Directory is one aggregated architecture directory.
type Directory = architectureDirectory

// Count is one named architecture count.
type Count = architectureCount

// Symbol is one source-linked architecture declaration.
type Symbol = architectureSymbol

// Relation is one directory relation.
type Relation = architectureRelation

// RelationEvidence is one source-linked relation fact.
type RelationEvidence = architectureRelationEvidence

// Comparison is a normalized architecture comparison.
type Comparison = architectureComparison

// ResolveOutput is the structured architecture symbol projection.
type ResolveOutput = architectureResolveOutput

// WhyOutput is the structured architecture relation projection.
type WhyOutput = architectureWhyOutput

// ResponsibilitiesOutput is the structured directory responsibility projection.
type ResponsibilitiesOutput = architectureResponsibilitiesOutput

// ResolveSymbols finds exact and terminal symbol matches.
func ResolveSymbols(symbols []Symbol, symbol string) []Symbol {
	return resolveArchitectureSymbols(symbols, symbol)
}

// RelationEvidenceFor selects source evidence between two directories.
func RelationEvidenceFor(relations []Relation, from, to string) []RelationEvidence {
	return architectureRelationEvidenceFor(relations, from, to)
}

// CleanDirectory normalizes one architecture directory identity.
func CleanDirectory(value string) string { return cleanArchitectureDirectory(value) }

// EvidenceRelation summarizes evidence relation kinds.
func EvidenceRelation(evidence []RelationEvidence) string {
	return architectureEvidenceRelation(evidence)
}

// BuildResponsibilities projects directory responsibilities.
func BuildResponsibilities(report Report) ResponsibilitiesOutput {
	return buildArchitectureResponsibilitiesOutput(report)
}

// Compare normalizes and compares two complete architecture reports.
func Compare(beforePath, afterPath string, beforeBytes, afterBytes []byte, before, after Report) Comparison {
	return compareDirectoryArchitectures(beforePath, afterPath, beforeBytes, afterBytes, before, after)
}

// Read decodes and validates one complete local or remote architecture report.
func Read(path string) (Report, []byte, error) { return readDirectoryArchitecture(path) }

// FormatCounts renders deterministic architecture count summaries.
func FormatCounts(counts []Count) string { return formatArchitectureCounts(counts) }

// Dependencies supplies parent-owned source configuration, remote transport, and exit state.
type Dependencies struct {
	ApplySourceConfig func(*search.Params) error
	Remote            func(context.Context, api.AnalysisRequest, string) (api.AnalysisResponse, error)
	ServerDefault     func(string) string
	RequestExit       func(int)
	Stdout            io.Writer
	Daemon            bool
}

type command struct {
	application  cliruntime.Context
	dependencies Dependencies
}

// New constructs the architecture command from the common command context.
func New(application cliruntime.Context) cliruntime.Command {
	return &command{application: application}
}

func newWithDependencies(dependencies Dependencies) cliruntime.Command {
	return &command{dependencies: dependencies}
}

func (command *command) services() Dependencies {
	if command.application == nil {
		return command.dependencies
	}
	application := command.application
	return Dependencies{
		ApplySourceConfig: search.SourcePolicyConfigurer(application.Repository()),
		Remote: func(ctx context.Context, request api.AnalysisRequest, server string) (api.AnalysisResponse, error) {
			invocation := application.Repository().InvocationOptions()
			request.ProductionOnly = request.ProductionOnly || invocation.ProductionOnly
			request.NoConfigIgnore = request.NoConfigIgnore || invocation.NoConfigIgnore
			request.NoRepoConfig = request.NoRepoConfig || invocation.NoRepositoryConfig
			response, err := application.APIClient().Analysis(ctx, server, request)
			if err != nil {
				return api.AnalysisResponse{}, err
			}
			for _, notice := range response.Notices {
				fmt.Fprintln(application.Stderr(), "analysis notice:", notice)
			}
			for _, shardError := range response.ShardErrors {
				fmt.Fprintln(application.Stderr(), "partial analysis:", shardError)
			}
			return response, nil
		},
		ServerDefault: application.Configuration().ServerDefault,
		RequestExit:   application.RequestExit,
		Stdout:        application.Stdout(),
	}
}

func (d Dependencies) applySourceConfig(params *search.Params) error {
	if d.ApplySourceConfig == nil {
		return nil
	}
	return d.ApplySourceConfig(params)
}
func (d Dependencies) remote(ctx context.Context, request api.AnalysisRequest, server string) (api.AnalysisResponse, error) {
	if d.Remote == nil {
		return api.AnalysisResponse{}, fmt.Errorf("remote architecture is unavailable")
	}
	return d.Remote(ctx, request, server)
}
func (d Dependencies) serverDefault(server string) string {
	if d.ServerDefault == nil {
		return server
	}
	return d.ServerDefault(server)
}
func (d Dependencies) requestExit(code int) {
	if d.RequestExit != nil {
		d.RequestExit(code)
	}
}

type outputWriter struct{ output *cliruntime.Output }

func outputDestination(dependencies Dependencies) io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
}
func stdoutWriter(dependencies Dependencies) *outputWriter {
	return &outputWriter{output: cliruntime.NewOutput(outputDestination(dependencies))}
}
func newBoundedOutputWriter(writer io.Writer, maxBytes int) *outputWriter {
	return &outputWriter{output: cliruntime.NewBoundedOutput(writer, maxBytes)}
}
func (w *outputWriter) writeString(value string) error { return w.output.WriteString(value) }
func (w *outputWriter) writeJSON(value any) error      { return w.output.WriteJSON(value) }

func compactNavigationSourceSummary(s SourceSummary) string {
	return fmt.Sprintf("discovered:%d,selected:%d,parsed:%d,skipped:%d,failed:%d,recovered:%d", s.Discovered, s.Selected, s.Parsed, s.Skipped, s.Failed, s.Recovered)
}
func navigationSourcePaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		capabilities, ok := parser.CapabilitiesForLanguage(parser.LanguageFor(path))
		if ok && capabilities.Navigation {
			result = append(result, path)
		}
	}
	return result
}
