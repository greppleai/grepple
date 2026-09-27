// Package api defines Grepple-controlled HTTP request and response contracts.
//
// It owns transport-only envelopes and aliases canonical domain models when the
// wire schema is identical. Validation, defaults, persistence, and algorithms
// belong to the domain packages that consume these contracts.
package api

import (
	"github.com/greppleai/grepple/navigation"
	"github.com/greppleai/grepple/rulespec"
	searchdomain "github.com/greppleai/grepple/search"
)

// SearchRequest is retained as a wire-compatible alias of the Search domain request.
type SearchRequest = searchdomain.Request

// ResultMatch is retained as a wire-compatible alias of the Search domain model.
type ResultMatch = searchdomain.ResultMatch

// ResultSegment is retained as a wire-compatible alias of the Search domain model.
type ResultSegment = searchdomain.ResultSegment

// ContextLine is retained as a wire-compatible alias of the Search domain model.
type ContextLine = searchdomain.ContextLine

// Navigation models remain API aliases so existing wire and library consumers
// preserve their source and JSON compatibility while algorithms use their owner.
type NavigationArtifactIdentity = navigation.ArtifactIdentity
type ExternalDependencyCandidate = navigation.ExternalDependencyCandidate
type ExternalNavigationReference = navigation.ExternalReference
type NavigationResolveRequest = navigation.ResolveRequest
type NavigationResolveResult = navigation.ResolveResult
type NavigationResolveResponse = navigation.ResolveResponse
type RelatedSymbol = navigation.RelatedSymbol

// Search result models remain aliases so existing API consumers preserve source
// and JSON compatibility while Search owns their behavior and representation.
type LineRangeResult = searchdomain.LineRangeResult
type FileResult = searchdomain.FileResult
type RepoCount = searchdomain.RepoCount

// SearchResponse is the on-the-wire search response.
type SearchResponse struct {
	Results        []FileResult    `json:"results"`
	Metadata       *ResultMetadata `json:"metadata,omitempty"`
	SourceAnalysis *SourceAnalysis `json:"sourceAnalysis,omitempty"`
	RepoCounts     []RepoCount     `json:"repoCounts,omitempty"`
	ShardErrors    []string        `json:"shardErrors,omitempty"`
	Truncated      bool            `json:"truncated,omitempty"`
}

// SourceAnalysis summarizes structural parsing for returned search results.
type SourceAnalysis struct {
	Returned    int `json:"returned"`
	Structured  int `json:"structured"`
	Plain       int `json:"plain"`
	Unsupported int `json:"unsupported"`
	Failed      int `json:"failed"`
	Recovered   int `json:"recovered"`
}

// RepoListEntry is one repository checkout returned by the public repository endpoint.
type RepoListEntry struct {
	Repo      string  `json:"repo"`
	Selector  string  `json:"selector,omitempty"`
	Shard     string  `json:"shard,omitempty"`
	Ref       string  `json:"ref,omitempty"`
	RefKind   string  `json:"refKind,omitempty"`
	Head      *string `json:"head,omitempty"`
	IndexedAt string  `json:"indexedAt,omitempty"`
}

// RepositoryIndexConfig declares additional repository refs that an indexed
// repository asks the backend to retain.
type RepositoryIndexConfig struct {
	Repositories []RepositoryIndexTarget `json:"repositories,omitempty"`
}

// RepositoryIndexTarget selects branches and tags from one GitHub repository.
// The backend always indexes the repository's default branch independently.
type RepositoryIndexTarget struct {
	Repo     string   `json:"repo"`
	Branches []string `json:"branches,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// ReposResponse is the payload of GET /public/repos.
type ReposResponse struct {
	OK    bool            `json:"ok"`
	Count int             `json:"count"`
	Repos []RepoListEntry `json:"repos"`
}

// TreeEntry is one path in a repository tree listing.
type TreeEntry struct {
	Path           string   `json:"path"`
	Dir            bool     `json:"dir"`
	Description    string   `json:"description,omitempty"`
	Areas          []string `json:"areas,omitempty"`
	MetadataStatus string   `json:"metadataStatus,omitempty"`
	MetadataIssues []string `json:"metadataIssues,omitempty"`
}

// TreeResponse is the tree endpoint payload.
type TreeResponse struct {
	Repo           string      `json:"repo"`
	Path           string      `json:"path"`
	Description    string      `json:"description,omitempty"`
	Areas          []string    `json:"areas,omitempty"`
	MetadataStatus string      `json:"metadataStatus,omitempty"`
	MetadataIssues []string    `json:"metadataIssues,omitempty"`
	Depth          int         `json:"depth"`
	Commit         *string     `json:"commit"`
	Entries        []TreeEntry `json:"entries"`
}

const (
	// RuleModeCount stores per-repository file and match tallies.
	RuleModeCount = rulespec.ModeCount
	// RuleModeFiles additionally stores matching file paths.
	RuleModeFiles = rulespec.ModeFiles
	// RuleEngineText identifies legacy literal or regular-expression rules.
	RuleEngineText = rulespec.EngineText
	// RuleEngineGritQL identifies native structural rules.
	RuleEngineGritQL = rulespec.EngineGritQL
)

// Rule is retained as a wire-compatible alias of the Rules domain model.
type Rule = rulespec.Rule

// RuleRepoResult is a rule's materialized result for one repository.
type RuleRepoResult struct {
	Repo        string   `json:"repo"`
	Files       int      `json:"files"`
	Matches     int      `json:"matches"`
	Paths       []string `json:"paths,omitempty"`
	Head        string   `json:"head,omitempty"`
	EvaluatedAt string   `json:"evaluatedAt,omitempty"`
}

// RuleResults is an aggregated cross-shard rule result.
type RuleResults struct {
	Rule       string           `json:"rule"`
	Mode       string           `json:"mode,omitempty"`
	Generation int64            `json:"generation,omitempty"`
	Repos      []RuleRepoResult `json:"repos"`
	Truncated  bool             `json:"truncated,omitempty"`
}

// RuleSet is a generation-stamped collection returned by the rules API.
type RuleSet struct {
	Generation int64  `json:"generation"`
	Rules      []Rule `json:"rules"`
}
