package architecture

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/greppleai/grepple/internal/analysis"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

const defaultTextOutputBytes = 16 * 1024

type commonArgs = cliruntime.CommonArgs

// SourceSummary describes source-universe processing.
type SourceSummary = analysis.SourceSummary

type Report = analysis.ArchitectureReport
type architectureSourceFile = analysis.ArchitectureSourceFile
type architectureDirectory = analysis.ArchitectureDirectory
type architectureCount = analysis.ArchitectureCount
type architectureSymbol = analysis.ArchitectureSymbol
type architectureRelation = analysis.ArchitectureRelation
type architectureRelationEvidence = analysis.ArchitectureRelationEvidence

type directoryArchitecture = Report

func isExtractHelp(value string) bool { return value == "--help" || value == "-h" || value == "help" }

// Schema identifies complete directory architecture reports.
const Schema = directoryArchitectureSchema

// SourceFile is one classified source in an architecture report.
type SourceFile = architectureSourceFile

// Directory is one aggregated architecture directory.
type Directory = architectureDirectory

// Symbol is one source-linked architecture declaration.
type Symbol = architectureSymbol

// Relation is one directory relation.
type Relation = architectureRelation

// RelationEvidence is one source-linked relation fact.
type RelationEvidence = architectureRelationEvidence

// Dependencies supplies parent-owned source configuration and remote transport.
type Dependencies struct {
	ApplySourceConfig func(*search.Params) error
	Remote            func(context.Context, wire.AnalysisRequest, string) (wire.AnalysisResponse, error)
	ServerDefault     func(string) string
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
		Remote: func(ctx context.Context, request wire.AnalysisRequest, server string) (wire.AnalysisResponse, error) {
			invocation := application.Repository().InvocationOptions()
			request.ProductionOnly = request.ProductionOnly || invocation.ProductionOnly
			request.NoConfigIgnore = request.NoConfigIgnore || invocation.NoConfigIgnore
			request.NoRepoConfig = request.NoRepoConfig || invocation.NoRepositoryConfig
			response, err := application.APIClient().Analysis(ctx, server, request)
			if err != nil {
				return wire.AnalysisResponse{}, err
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
		Stdout:        application.Stdout(),
	}
}

func (d Dependencies) applySourceConfig(params *search.Params) error {
	if d.ApplySourceConfig == nil {
		return nil
	}
	return d.ApplySourceConfig(params)
}
func (d Dependencies) remote(ctx context.Context, request wire.AnalysisRequest, server string) (wire.AnalysisResponse, error) {
	if d.Remote == nil {
		return wire.AnalysisResponse{}, fmt.Errorf("remote architecture is unavailable")
	}
	return d.Remote(ctx, request, server)
}
func (d Dependencies) serverDefault(server string) string {
	if d.ServerDefault == nil {
		return server
	}
	return d.ServerDefault(server)
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
