package search

import (
	"fmt"
	"grepple/internal/api"
	"strconv"
)

// ResolveRequest merges a wire api.SearchRequest into validated Params: unset optional
// fields keep their defaults, and a content search must carry a query.
func ResolveRequest(r api.SearchRequest) (Params, error) {
	p := Params{Regex: true, MaxSegments: DefaultMaxSegments, Files: r.Files}
	if r.Query != nil {
		p.Query = *r.Query
	}
	if !p.Files && p.Query == "" {
		return p, fmt.Errorf("search request requires a non-empty 'query'")
	}
	applyOptionalFields(&p, r)
	p.Context = resolveContext(r.Context)
	p.SkipSegments = r.SkipSegments
	if r.CountByRepo {
		// A count probe never needs structural segments.
		p.CountByRepo = true
		p.SkipSegments = true
	}
	return p, nil
}

// applyOptionalFields copies the request's explicitly set fields into p;
// unset (nil/zero) fields keep their defaults.
func applyOptionalFields(p *Params, r api.SearchRequest) {
	if len(r.Globs) > 0 {
		p.Globs = r.Globs
	}
	if r.Regex != nil {
		p.Regex = *r.Regex
	}
	if r.IgnoreCase != nil {
		p.IgnoreCase = *r.IgnoreCase
	}
	if r.MaxFiles != nil && *r.MaxFiles >= 1 {
		p.MaxFiles = *r.MaxFiles
	}
	if r.MaxSegments != nil && *r.MaxSegments >= 1 {
		p.MaxSegments = *r.MaxSegments
	}
	if r.Skip != nil && *r.Skip > 0 {
		p.Skip = *r.Skip
	}
	if r.Limit != nil && *r.Limit >= 1 {
		p.Limit = *r.Limit
	}
	p.Repo = repoPatterns(r.Repo)
	p.ExcludeRepo = repoPatterns(r.ExcludeRepo)
}

// resolveContext interprets the loosely typed context option (JSON numbers decode
// as float64, but numeric strings are accepted too); non-positive or
// unparseable values mean no context.
func resolveContext(value any) int {
	switch v := value.(type) {
	case float64:
		if v > 0 && v == float64(int(v)) {
			return int(v)
		}
	case string:
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			return n
		}
	}
	return 0
}

// DefaultPageLimit caps a response page when the client does not say how many
// result files it wants.
const DefaultPageLimit = 20

// MaxPageLimit is the hard server-side ceiling for one response page. Clients
// cannot override it: an unset limit resolves to DefaultPageLimit and an
// explicit 0 ("all") or oversized limit is clamped down to MaxPageLimit.
// Larger result sets are paged through with skip.
const MaxPageLimit = 100

// EnforcePageLimit applies the public paging policy to params resolved from a
// client request. Call it ONLY at the client trust boundary (the router's
// public endpoints): internal fan-out requests legitimately carry skip+limit
// windows above the cap and must never be clamped, or deep pages would lose
// results. The shard's in-cluster /search endpoint stays uncapped for that
// reason.
func EnforcePageLimit(p *Params, r api.SearchRequest) {
	switch {
	case r.Limit == nil:
		p.Limit = DefaultPageLimit
	case *r.Limit < 1 || *r.Limit > MaxPageLimit:
		p.Limit = MaxPageLimit
	}
}

// RepoMatches reports whether repo matches any include pattern (no patterns
// matches everything). Exported so the shard can pre-filter a shard's repos
// with the exact same semantics the search engine applies per file.
func RepoMatches(repo string, patterns []string) bool { return repoMatches(repo, patterns) }

// RepoMatchesAny reports whether repo matches any of the (exclude) patterns.
func RepoMatchesAny(repo string, patterns []string) bool { return repoMatchesAny(repo, patterns) }

func repoPatterns(value any) []string {
	var patterns []string
	switch typed := value.(type) {
	case string:
		if typed != "" {
			patterns = append(patterns, typed)
		}
	case []any:
		for _, item := range typed {
			if pattern, ok := item.(string); ok && pattern != "" {
				patterns = append(patterns, pattern)
			}
		}
	case []string:
		for _, pattern := range typed {
			if pattern != "" {
				patterns = append(patterns, pattern)
			}
		}
	}
	return patterns
}
