// Package wire owns transport contracts shared by the public API and internal adapters.
package wire

import (
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/rulespec"
	searchdomain "github.com/greppleai/grepple/internal/search"
)

// SearchRequest is retained as a wire-compatible alias of the Search domain request.
type SearchRequest = searchdomain.Request

// ResultMatch is retained as a wire-compatible alias of the Search domain model.
type ResultMatch = searchdomain.ResultMatch

// ResultSegment is retained as a wire-compatible alias of the Search domain model.
type ResultSegment = searchdomain.ResultSegment

// ContextLine is retained as a wire-compatible alias of the Search domain model.
type ContextLine = searchdomain.ContextLine

// NavigationArtifactIdentity pins a symbol to immutable indexed source.
type NavigationArtifactIdentity = navigation.ArtifactIdentity

// ExternalDependencyCandidate is one exact manifest or lockfile identity.
type ExternalDependencyCandidate = navigation.ExternalDependencyCandidate

// ExternalNavigationReference carries source-written unresolved symbol evidence.
type ExternalNavigationReference = navigation.ExternalReference

// NavigationResolveRequest batches external references for artifact resolution.
type NavigationResolveRequest = navigation.ResolveRequest

// NavigationResolveResult correlates a reference with candidate symbols.
type NavigationResolveResult = navigation.ResolveResult

// NavigationResolveResponse returns indexed dependency symbol candidates.
type NavigationResolveResponse = navigation.ResolveResponse

// RelatedSymbol is a source-linked declaration with optional artifact evidence.
type RelatedSymbol = navigation.RelatedSymbol

// LineRangeResult reports the returned inclusive range and any EOF clamp.
type LineRangeResult = searchdomain.LineRangeResult

// FileResult is one source file and its source-backed search results.
type FileResult = searchdomain.FileResult

// RepoCount is a per-repository tally of matching files and lines.
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
