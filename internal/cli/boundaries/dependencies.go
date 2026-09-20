package boundaries

import (
	"context"
	"fmt"
	"os"

	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type commonArgs = cliruntime.CommonArgs

// SourceSummary describes source-universe processing.
type SourceSummary struct {
	Discovered int `json:"discovered"`
	Selected   int `json:"selected"`
	Parsed     int `json:"parsed"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
	Recovered  int `json:"recovered"`
}

// Truncation describes a source-universe limit.
type Truncation struct {
	Reason  string `json:"reason"`
	Limit   int    `json:"limit"`
	Skipped int    `json:"skipped"`
}

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
	ResolvePaths   func([]string) ([]string, error)
	BuildGraph     func([]string, int, search.NavigationBuildOptions) GraphOutput
	CacheDirectory func() string
	Remote         func(context.Context, api.AnalysisRequest, string) (api.AnalysisResponse, error)
	ServerDefault  func(string) string
	Metadata       func(MetadataInput) *api.ResultMetadata
}

func (d Dependencies) resolvePaths(paths []string) ([]string, error) {
	if d.ResolvePaths == nil {
		return nil, fmt.Errorf("boundary source resolution is unavailable")
	}
	return d.ResolvePaths(paths)
}
func (d Dependencies) buildGraph(paths []string, maxFiles int, options search.NavigationBuildOptions) GraphOutput {
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
	if d.Metadata == nil {
		return nil
	}
	return d.Metadata(input)
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
