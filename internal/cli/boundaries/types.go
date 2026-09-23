package boundaries

import (
	"context"
	"fmt"
	"os"

	"github.com/greppleai/grepple/analysis"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/navigation"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type commonArgs = cliruntime.CommonArgs

// SourceSummary describes source-universe processing.
type SourceSummary = analysis.SourceSummary

// Truncation describes a source-universe limit.
type Truncation = analysis.Truncation

type boundariesOutput = Report

// GraphOutput contains navigation facts consumed by boundary analysis.
type GraphOutput struct {
	Files          int                             `json:"files"`
	Sources        SourceSummary                   `json:"sources"`
	Declarations   []parser.NavigationDeclaration  `json:"declarations"`
	Calls          []parser.NavigationCall         `json:"calls"`
	Fields         []parser.NavigationField        `json:"fields,omitempty"`
	TypeUsages     []parser.NavigationTypeUsage    `json:"typeUsages,omitempty"`
	MemberAccesses []parser.NavigationMemberAccess `json:"memberAccesses,omitempty"`
	Truncation     *Truncation                     `json:"truncation,omitempty"`
}

// MetadataInput contains complete report and command limits for result metadata.
type MetadataInput struct {
	JSON                                            bool
	Paths                                           []string
	MinOccurrences, MaxFiles, Limit, MaxOutputBytes int
	Policy                                          string
	Report                                          Report
}

// Dependencies supplies shared navigation, storage, remote transport, and metadata services.
type Dependencies struct {
	ResolvePaths     func([]string) ([]string, error)
	BuildGraph       func([]string, int, navigation.BuildOptions) GraphOutput
	CacheDirectory   func() string
	Remote           func(context.Context, api.AnalysisRequest, string) (api.AnalysisResponse, error)
	ServerDefault    func(string) string
	ActiveScopeFlags func([]string) []string
}

type command struct{ dependencies Dependencies }

// New constructs the boundaries command from the common command context.
func New(application cliruntime.Context) cliruntime.Command {
	return newWithDependencies(Dependencies{
		ResolvePaths: func(paths []string) ([]string, error) {
			params := search.Params{Files: true, Globs: paths}
			if err := search.ConfigureSourcePolicy(&params, application.Repository()); err != nil {
				return nil, err
			}
			return search.ListFilePaths(params, nil)
		},
		BuildGraph: func(paths []string, maxFiles int, options navigation.BuildOptions) GraphOutput {
			universe, err := analysis.NewUniverseWithOptions(analysis.ReadSources(paths), maxFiles, options)
			if err != nil {
				return GraphOutput{}
			}
			defer universe.Close()
			report, err := analysis.BuildGraph(universe, nil)
			if err != nil {
				return GraphOutput{}
			}
			return GraphFromAnalysis(report)
		},
		CacheDirectory: application.Repository().CacheDirectory,
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
			if !response.Complete {
				fmt.Fprintln(application.Stderr(), "analysis is incomplete; inspect result source counts and truncation")
			}
			return response, nil
		},
		ServerDefault:    application.Configuration().ServerDefault,
		ActiveScopeFlags: application.Repository().AppendScopeFlags,
	})
}

func newWithDependencies(dependencies Dependencies) cliruntime.Command {
	return &command{dependencies: dependencies}
}

func (d Dependencies) resolvePaths(paths []string) ([]string, error) {
	if d.ResolvePaths == nil {
		return nil, fmt.Errorf("boundary source resolution is unavailable")
	}
	return d.ResolvePaths(paths)
}
func (d Dependencies) buildGraph(paths []string, maxFiles int, options navigation.BuildOptions) GraphOutput {
	return d.BuildGraph(paths, maxFiles, options)
}
func (d Dependencies) cacheDirectory() string {
	if d.CacheDirectory == nil {
		return ""
	}
	return d.CacheDirectory()
}
func (d Dependencies) remote(ctx context.Context, request api.AnalysisRequest, server string) (api.AnalysisResponse, error) {
	if d.Remote == nil {
		return api.AnalysisResponse{}, fmt.Errorf("remote boundaries are unavailable")
	}
	return d.Remote(ctx, request, server)
}
func (d Dependencies) serverDefault(server string) string {
	if d.ServerDefault == nil {
		return server
	}
	return d.ServerDefault(server)
}
func (d Dependencies) metadata(input MetadataInput) *api.ResultMetadata {
	return resultMetadata(input, d.ActiveScopeFlags)
}

type outputWriter struct{ output *cliruntime.Output }

func stdoutWriter() *outputWriter { return &outputWriter{output: cliruntime.NewOutput(os.Stdout)} }
func newBoundedOutputWriter(writer *os.File, maxBytes int) *outputWriter {
	return &outputWriter{output: cliruntime.NewBoundedOutput(writer, maxBytes)}
}
func (w *outputWriter) writeString(value string) error { return w.output.WriteString(value) }
func (w *outputWriter) writeJSON(value any) error      { return w.output.WriteJSON(value) }

// BuildCachedGraph builds or restores the graph used for boundary analysis.
func BuildCachedGraph(paths []string, maxFiles int, useCache bool, dependencies Dependencies) (GraphOutput, string, error) {
	return buildCachedBoundaryGraph(paths, maxFiles, useCache, dependencies)
}
