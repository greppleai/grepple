package search

import (
	"context"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/wire"
)

// SearchRemote executes a remote search through the application API client.
func SearchRemote(application cliruntime.Context, options *Options, server string) ([]wire.FileResult, error) {
	return searchRemote(application, options, server)
}

// ResolveNavigation resolves imported navigation references through the application API client.
func ResolveNavigation(application cliruntime.Context, ctx context.Context, request wire.NavigationResolveRequest, server string) (wire.NavigationResolveResponse, error) {
	return requestNavigationResolve(application, ctx, request, server)
}
