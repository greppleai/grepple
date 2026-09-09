// Package api defines Grepple-controlled HTTP request and response contracts.
//
// It intentionally contains only dependency-free data types and constants. Wire
// validation, defaults, transport behavior, persistence, and application logic
// belong to the packages that consume these contracts.
package api

// SearchRequest is the on-the-wire search request. Nil pointer fields mean
// "unset", allowing each HTTP surface to apply its defaults.
type SearchRequest struct {
	Query         *string  `json:"query,omitempty"`
	Globs         []string `json:"globs,omitempty"`
	Regex         *bool    `json:"regex,omitempty"`
	IgnoreCase    *bool    `json:"ignoreCase,omitempty"`
	InvertMatch   *bool    `json:"invertMatch,omitempty"`
	MaxFiles      *int     `json:"maxFiles,omitempty"`
	MaxSegments   *int     `json:"maxSegments,omitempty"`
	Skip          *int     `json:"skip,omitempty"`
	Limit         *int     `json:"limit,omitempty"`
	Repo          any      `json:"repo,omitempty"`
	ExcludeRepo   any      `json:"excludeRepo,omitempty"`
	Files         bool     `json:"files,omitempty"`
	Context       any      `json:"context,omitempty"`
	BeforeContext *int     `json:"beforeContext,omitempty"`
	AfterContext  *int     `json:"afterContext,omitempty"`
	SkipSegments  bool     `json:"skipSegments,omitempty"`
	CountByRepo   bool     `json:"countByRepo,omitempty"`
}

// ResultMatch is one matching line inside a file.
type ResultMatch struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// ResultSegment is one structural segment around a file's matches.
type ResultSegment struct {
	Kind  string `json:"kind"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// ContextLine is one line of match context.
type ContextLine struct {
	Line  int    `json:"line"`
	Text  string `json:"text"`
	Match bool   `json:"match"`
}

// FileResult is one matching file.
type FileResult struct {
	Path     string          `json:"path"`
	Repo     string          `json:"repo,omitempty"`
	Language string          `json:"language"`
	Matches  []ResultMatch   `json:"matches"`
	Segments []ResultSegment `json:"segments"`
	Context  []ContextLine   `json:"context,omitempty"`
}

// RepoCount is a per-repository tally of matching files and lines.
type RepoCount struct {
	Repo    string `json:"repo"`
	Files   int    `json:"files"`
	Matches int    `json:"matches"`
}

// SearchResponse is the on-the-wire search response.
type SearchResponse struct {
	Results     []FileResult `json:"results"`
	RepoCounts  []RepoCount  `json:"repoCounts,omitempty"`
	ShardErrors []string     `json:"shardErrors,omitempty"`
	Truncated   bool         `json:"truncated,omitempty"`
}

// RepoRef identifies a repository to index.
type RepoRef struct {
	Repo string `json:"repo"`
	URL  string `json:"url,omitempty"`
	Ref  string `json:"ref,omitempty"`
}

// RepoInfo describes one indexed repository and its freshness metadata.
type RepoInfo struct {
	Repo      string  `json:"repo"`
	URL       string  `json:"url"`
	Ref       string  `json:"ref"`
	Dir       string  `json:"dir"`
	Head      *string `json:"head"`
	IndexedAt string  `json:"indexedAt"`
}

// RepoListEntry is one repository returned by the public repository endpoint.
type RepoListEntry struct {
	Repo      string  `json:"repo"`
	Shard     string  `json:"shard,omitempty"`
	Ref       string  `json:"ref,omitempty"`
	Head      *string `json:"head,omitempty"`
	IndexedAt string  `json:"indexedAt,omitempty"`
}

// ReposResponse is the payload of GET /public/repos.
type ReposResponse struct {
	OK    bool            `json:"ok"`
	Count int             `json:"count"`
	Repos []RepoListEntry `json:"repos"`
}

// TreeEntry is one path in a repository tree listing.
type TreeEntry struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// TreeResponse is the tree endpoint payload.
type TreeResponse struct {
	Repo    string      `json:"repo"`
	Path    string      `json:"path"`
	Depth   int         `json:"depth"`
	Commit  *string     `json:"commit"`
	Entries []TreeEntry `json:"entries"`
}

const (
	// RuleModeCount stores per-repository file and match tallies.
	RuleModeCount = "count"
	// RuleModeFiles additionally stores matching file paths.
	RuleModeFiles = "files"
)

// Rule is a saved search materialized per repository.
type Rule struct {
	ID        string        `json:"id"`
	Name      string        `json:"name,omitempty"`
	Mode      string        `json:"mode"`
	Request   SearchRequest `json:"request"`
	CreatedAt string        `json:"createdAt,omitempty"`
	UpdatedAt string        `json:"updatedAt,omitempty"`
}

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

// RuleSet is the versioned collection pushed from the router to shards.
type RuleSet struct {
	Generation int64  `json:"generation"`
	Rules      []Rule `json:"rules"`
}

// IndexListResponse is a shard's answer to GET /index.
type IndexListResponse struct {
	Repos []RepoInfo `json:"repos"`
}
