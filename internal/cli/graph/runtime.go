package graph

import (
	"context"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

// Services supplies invocation-scoped graph I/O and repository policy.
type Services struct {
	ApplySourceConfig func(*search.Params) error
	Remote            func(context.Context, api.AnalysisRequest, string) (api.AnalysisResponse, error)
	ServerDefault     func(string) string
	ActiveScopeFlags  func([]string) []string
	Metadata          func(MetadataInput) *api.ResultMetadata
	DiffMetadata      func(DiffMetadataInput) *api.ResultMetadata
}

// MetadataInput describes one graph result page.
type MetadataInput struct {
	Paths                              []string
	Returned, MaxFiles, MaxOutputBytes int
	JSON                               bool
	Sources                            SourceSummary
	Truncation                         *Truncation
	NextCommand                        string
}

// DiffMetadataInput describes one graph diff result.
type DiffMetadataInput struct {
	BeforePath, AfterPath    string
	MaxFiles, MaxOutputBytes int
	JSON                     bool
	Before, After            Output
	Diff                     search.NavigationGraphDiff
}

// ContinuationCommand builds the uncapped continuation for a truncated graph.
func ContinuationCommand(mode string, paths []string, truncation *Truncation, services Services) string {
	return graphContinuationCommand(mode, paths, truncation, services)
}

func (services Services) server(value string) string {
	if services.ServerDefault == nil {
		return value
	}
	return services.ServerDefault(value)
}
