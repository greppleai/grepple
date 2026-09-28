package search

import (
	"context"
	"fmt"
	"time"

	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/linerange"
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func searchRemote(application cliruntime.Context, options *Options, server string) ([]wire.FileResult, error) {
	return searchRemoteContext(application, context.Background(), options, server)
}

func searchRemoteContext(application cliruntime.Context, ctx context.Context, options *Options, server string) ([]wire.FileResult, error) {
	result, err := application.APIClient().Search(ctx, server, searchRequestFromParams(options.Params))
	if err != nil {
		if apiclient.RangeOutcome(err) == linerange.OutcomeFullMiss {
			render.RecordStandaloneLineRangeOutcome(linerange.OutcomeFullMiss, application.Configuration().ContextGuardEnabled())
			return nil, remoteFullLineRangeMissError{err: err}
		}
		return nil, err
	}
	warnPartialResults(application, result)
	return dropExcludedRepos(result.Results, options.Params.ExcludeRepo), nil
}

func resolveLocalExternalNavigation(application cliruntime.Context, results []wire.FileResult, server string) []wire.FileResult {
	workingDirectory := application.Repository().WorkingDirectory()
	if err := navigation.NewExternalDependencyService[wire.FileResult]().QualifyLocal(results, workingDirectory); err != nil {
		fmt.Fprintf(application.Stderr(), "warning: dependency navigation evidence failed: %v\n", err)
		return results
	}
	references := navigation.NewExternalDependencyService[wire.FileResult]().References(results)
	if len(references) == 0 {
		return results
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := requestNavigationResolve(application, ctx, wire.NavigationResolveRequest{References: references}, application.Configuration().ServerDefault(server))
	if err != nil {
		fmt.Fprintf(application.Stderr(), "warning: dependency navigation lookup failed: %v\n", err)
		return results
	}
	return navigation.NewExternalResolutionService[wire.FileResult]().Apply(results, response)
}

func requestNavigationResolve(application cliruntime.Context, ctx context.Context, request wire.NavigationResolveRequest, server string) (wire.NavigationResolveResponse, error) {
	return application.APIClient().ResolveNavigation(ctx, server, request)
}

// searchRequestFromParams converts resolved CLI parameters into the wire
// request: zero values stay unset so the server applies its own defaults.
func searchRequestFromParams(params search.Params) wire.SearchRequest {
	return search.RequestFromParams(params)
}

// warnPartialResults reports shard failures and index-cap truncation for a
// distributed search response on stderr.
func warnPartialResults(application cliruntime.Context, result wire.SearchResponse) {
	if len(result.ShardErrors) > 0 {
		fmt.Fprintf(application.Stderr(), "warning: partial results — %d shard(s) unavailable:\n", len(result.ShardErrors))
		for _, shardErr := range result.ShardErrors {
			fmt.Fprintf(application.Stderr(), "  - %s\n", shardErr)
		}
	}
	if result.Truncated {
		fmt.Fprintln(application.Stderr(), "note: results truncated (index result cap hit) — narrow with --repo or a more specific pattern for the complete set")
	}
}

// dropExcludedRepos removes results from excluded repositories client-side,
// behind the server-side filter.
func dropExcludedRepos(results []wire.FileResult, exclude []string) []wire.FileResult {
	if len(exclude) == 0 {
		return results
	}
	filtered := results[:0]
	for _, item := range results {
		if !contains(exclude, item.Repo) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// countRemote fetches per-repository match tallies from the router. It sends an
// unbounded CountByRepo probe (no paging window, no segments) so the counts are
// complete yet the payload is tiny regardless of how many files match.
func countRemote(application cliruntime.Context, options *Options, server string) ([]wire.RepoCount, error) {
	params := options.Params
	query := params.Query
	request := wire.SearchRequest{
		Query: &query, Globs: params.Globs, Regex: &params.Regex, IgnoreCase: &params.IgnoreCase,
		InvertMatch: &params.InvertMatch, CountByRepo: true, SkipSegments: true,
	}
	if len(params.Repo) > 0 {
		request.Repo = params.Repo
	}
	if len(params.ExcludeRepo) > 0 {
		request.ExcludeRepo = params.ExcludeRepo
	}
	result, err := application.APIClient().Search(context.Background(), server, request)
	if err != nil {
		return nil, err
	}
	if len(result.ShardErrors) > 0 {
		fmt.Fprintf(application.Stderr(), "warning: partial counts — %d shard(s) unavailable:\n", len(result.ShardErrors))
		for _, shardErr := range result.ShardErrors {
			fmt.Fprintf(application.Stderr(), "  - %s\n", shardErr)
		}
	}
	if result.Truncated {
		fmt.Fprintln(application.Stderr(), "note: counts truncated (index result cap hit) — narrow with --repo or a more specific pattern for exact totals")
	}
	return result.RepoCounts, nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
