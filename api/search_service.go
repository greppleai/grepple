package api

import (
	"context"

	"github.com/greppleai/grepple/internal/search"
)

// SearchPlan is an immutable resolved execution plan. Engine parameters stay
// private; callers can inspect or derive a new plan using value options.
type SearchPlan interface {
	Options() SearchPlanOptions
}

type searchPlan struct{ value search.Params }

func (plan searchPlan) Options() SearchPlanOptions {
	return SearchPlanOptions(cloneSearchParams(plan.value))
}
func searchParamsFor(plan SearchPlan) search.Params { return search.Params(plan.Options()) }

// SearchPlanOptions is the value configuration accepted by NewSearchPlan.
// It is not an execution handle or an HTTP wire request.
type SearchPlanOptions struct {
	Query           string
	Globs           []string
	Regex           bool
	IgnoreCase      bool
	InvertMatch     bool
	MaxFiles        int
	Skip            int
	Limit           int
	Sort            string
	Repo            []string
	ExcludeRepo     []string
	Files           bool
	Context         int
	BeforeContext   int
	AfterContext    int
	SkipSegments    bool
	LineRanges      bool
	EnclosingRanges bool
	Related         bool
	FollowRelated   int
	// RelatedRepositoryContext obtains complete matched-repository navigation
	// when the supplied file candidates contain only text-index matches.
	RelatedRepositoryContext bool
	NoRelated                bool
	At                       string
	Root                     string
	IgnorePaths              []string
	IgnoreRoot               string
	ProductionOnly           bool
	CountByRepo              bool
}

// NewSearchPlan creates a plan from explicit value options without sharing slices.
func NewSearchPlan(options SearchPlanOptions) SearchPlan {
	return searchPlan{value: cloneSearchParams(search.Params(options))}
}

func cloneSearchParams(params search.Params) search.Params {
	params.Globs = append([]string(nil), params.Globs...)
	params.Repo = append([]string(nil), params.Repo...)
	params.ExcludeRepo = append([]string(nil), params.ExcludeRepo...)
	params.IgnorePaths = append([]string(nil), params.IgnorePaths...)
	return params
}

// ResolveSearch validates the wire request and applies search defaults.
func ResolveSearch(request SearchRequest) (SearchPlan, error) {
	params, err := search.ResolveRequest(request)
	if err != nil {
		return nil, err
	}
	return searchPlan{value: cloneSearchParams(params)}, nil
}

// DefaultSearchPageLimit is the implicit public page size.
const DefaultSearchPageLimit = search.DefaultPageLimit

// MaxSearchPageLimit is the hard limit on a public response page.
const MaxSearchPageLimit = search.MaxPageLimit

// EnforceSearchPageLimit returns a new plan with the public boundary's limit.
// The router's internal fan-out windows remain unaffected.
func EnforceSearchPageLimit(plan SearchPlan, request SearchRequest) SearchPlan {
	params := searchParamsFor(plan)
	search.EnforcePageLimit(&params, request)
	return searchPlan{value: params}
}

// SearchBatch provides match counts without exposing engine-owned match data.
type SearchBatch interface {
	Len() int
	Results(before, after int, segments bool) []FileResult
	RepoCounts() []RepoCount
}

type searchBatch struct{ matches []search.FileMatch }

func (batch searchBatch) Len() int { return len(batch.matches) }
func (batch searchBatch) Results(before, after int, segments bool) []FileResult {
	return search.BuildResults(batch.matches, before, after, segments)
}
func (batch searchBatch) RepoCounts() []RepoCount {
	return search.AggregateRepoCounts(batch.matches)
}

// SearchFiles scans selected candidate paths, or all eligible sources if nil.
func SearchFiles(plan SearchPlan, candidates []string) (SearchBatch, error) {
	matches, err := search.Files(searchParamsFor(plan), candidates)
	return searchBatch{matches: matches}, err
}

// SearchAt resolves one exact source range with the same result projection.
func SearchAt(plan SearchPlan) (SearchBatch, error) {
	match, err := search.At(searchParamsFor(plan))
	if err != nil {
		return nil, err
	}
	return searchBatch{matches: []search.FileMatch{*match}}, nil
}

// ProjectSearchResults converts matches into stable wire-facing results.
func ProjectSearchResults(batch SearchBatch, before, after int, segments bool) []FileResult {
	return batch.Results(before, after, segments)
}

// SearchRepoCounts counts all matches without applying a result window.
func SearchRepoCounts(batch SearchBatch) []RepoCount {
	return batch.RepoCounts()
}

// ListSearchPaths discovers selected files; candidates constrain the walk.
func ListSearchPaths(plan SearchPlan, candidates []string) ([]string, error) {
	return search.ListFilePaths(searchParamsFor(plan), candidates)
}

// ListSearchPathsContext discovers selected files with cancellation.
func ListSearchPathsContext(ctx context.Context, plan SearchPlan, candidates []string) ([]string, error) {
	return search.ListFilePathsContext(ctx, searchParamsFor(plan), candidates)
}

// CollectSearchFilesUnder enumerates fallback files below indexed roots.
func CollectSearchFilesUnder(directories []string, root string) ([]string, error) {
	return search.CollectFilesUnder(directories, root)
}

// SearchRepoFilter tests indexed repository selectors without exposing matcher state.
type SearchRepoFilter interface{ Allow(repo string) bool }

type searchRepoFilter struct{ filter *search.RepoFilter }

// NewSearchRepoFilter compiles repository selector patterns.
func NewSearchRepoFilter(include, exclude []string) SearchRepoFilter {
	return searchRepoFilter{filter: search.NewRepoFilter(include, exclude)}
}

func (filter searchRepoFilter) Allow(repo string) bool {
	if filter.filter == nil {
		return false
	}
	return filter.filter.Allow(repo)
}

// SearchRepoID extracts the repository selector from a source path.
func SearchRepoID(path string) string { return search.RepoID(path) }

// SortSearchRepoCounts sorts count results deterministically.
func SortSearchRepoCounts(counts []RepoCount) { search.SortRepoCounts(counts) }

// SearchPathMatchesGlobs reports whether a candidate path matches the search globs.
func SearchPathMatchesGlobs(path string, globs []string) bool {
	return search.PathMatchesGlobs(path, globs)
}

// SplitSourceLines applies editor-style line counting to source text.
func SplitSourceLines(content string) []string { return search.SplitLines(content) }
