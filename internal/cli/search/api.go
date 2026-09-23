package search

import (
	"context"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
	searchengine "github.com/greppleai/grepple/search"
)

// ParseArgs parses search-owned command arguments.
func ParseArgs(application cliruntime.Context, args []string) (*Options, string, bool, error) {
	return parseSearchArgs(application, args)
}

// SearchRequestFromParams projects local parameters onto the remote API request.
func SearchRequestFromParams(params searchengine.Params) api.SearchRequest {
	return searchRequestFromParams(params)
}

// SearchRemote executes a remote search through the application API client.
func SearchRemote(application cliruntime.Context, options *Options, server string) ([]api.FileResult, error) {
	return searchRemote(application, options, server)
}

// SearchRemoteContext executes a remote search with caller-owned cancellation.
func SearchRemoteContext(application cliruntime.Context, ctx context.Context, options *Options, server string) ([]api.FileResult, error) {
	return searchRemoteContext(application, ctx, options, server)
}

// ResolveNavigation resolves imported navigation references through the application API client.
func ResolveNavigation(application cliruntime.Context, ctx context.Context, request api.NavigationResolveRequest, server string) (api.NavigationResolveResponse, error) {
	return requestNavigationResolve(application, ctx, request, server)
}
