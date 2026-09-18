// Package api defines Grepple-controlled HTTP request and response contracts.
//
// It intentionally contains only dependency-free data types and constants. Wire
// validation, defaults, transport behavior, persistence, and application logic
// belong to the packages that consume these contracts.
package api

// SearchRequest is the on-the-wire search request. Nil pointer fields mean
// "unset", allowing each HTTP surface to apply its defaults.
type SearchRequest struct {
	Query           *string  `json:"query,omitempty"`
	Globs           []string `json:"globs,omitempty"`
	Regex           *bool    `json:"regex,omitempty"`
	IgnoreCase      *bool    `json:"ignoreCase,omitempty"`
	InvertMatch     *bool    `json:"invertMatch,omitempty"`
	MaxFiles        *int     `json:"maxFiles,omitempty"`
	Skip            *int     `json:"skip,omitempty"`
	Limit           *int     `json:"limit,omitempty"`
	Sort            string   `json:"sort,omitempty"`
	Repo            any      `json:"repo,omitempty"`
	ExcludeRepo     any      `json:"excludeRepo,omitempty"`
	Files           bool     `json:"files,omitempty"`
	Context         any      `json:"context,omitempty"`
	BeforeContext   *int     `json:"beforeContext,omitempty"`
	AfterContext    *int     `json:"afterContext,omitempty"`
	SkipSegments    bool     `json:"skipSegments,omitempty"`
	LineRanges      bool     `json:"matchLineRanges,omitempty"`
	EnclosingRanges bool     `json:"enclosingLineRanges,omitempty"`
	Related         bool     `json:"related,omitempty"`
	NoRelated       bool     `json:"noRelated,omitempty"`
	At              string   `json:"at,omitempty"`
	FollowRelated   int      `json:"followRelated,omitempty"`
	CountByRepo     bool     `json:"countByRepo,omitempty"`
}

// ResultMatch is one matching line inside a file.
type ResultMatch struct {
	Line      int    `json:"line"`
	StartLine int    `json:"startLine,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	Text      string `json:"text"`
}

// ResultSegment is one structural segment around a file's matches. Kind "spacing"
// preserves a short whitespace-only gap without a verbose omission marker.
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

// NavigationArtifactIdentity pins a related declaration to one immutable dependency source artifact.
type NavigationArtifactIdentity struct {
	Ecosystem  string `json:"ecosystem"`
	Module     string `json:"module"`
	Version    string `json:"version"`
	RefKind    string `json:"refKind,omitempty"`
	Integrity  string `json:"integrity,omitempty"`
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

// ExternalDependencyCandidate is one exact manifest/lockfile identity that may
// provide an externally referenced symbol. Multiple candidates preserve JVM
// package-to-artifact ambiguity until indexed source declarations disambiguate it.
type ExternalDependencyCandidate struct {
	Ecosystem string `json:"ecosystem"`
	Module    string `json:"module"`
	Version   string `json:"version"`
	Integrity string `json:"integrity,omitempty"`
}

// ExternalNavigationReference retains syntax evidence needed to resolve one dependency symbol.
type ExternalNavigationReference struct {
	ID              string                        `json:"id"`
	Language        string                        `json:"language"`
	ImportPath      string                        `json:"importPath"`
	Package         string                        `json:"package,omitempty"`
	Symbol          string                        `json:"symbol"`
	ConsumerPackage string                        `json:"consumerPackage,omitempty"`
	ReceiverType    string                        `json:"receiverType,omitempty"`
	Kind            string                        `json:"kind"`
	Module          string                        `json:"module,omitempty"`
	Version         string                        `json:"version,omitempty"`
	Integrity       string                        `json:"integrity,omitempty"`
	Candidates      []ExternalDependencyCandidate `json:"candidates,omitempty"`
}

// NavigationResolveRequest batches exact dependency references for shard-local artifact lookup.
type NavigationResolveRequest struct {
	References []ExternalNavigationReference `json:"references"`
}

// NavigationResolveResult preserves correlation between a reference and exact declaration candidates.
type NavigationResolveResult struct {
	ID      string          `json:"id"`
	Symbols []RelatedSymbol `json:"symbols,omitempty"`
}

// NavigationResolveResponse is returned by a shard's immutable navigation artifact index.
type NavigationResolveResponse struct {
	Results []NavigationResolveResult `json:"results"`
}

// RelatedSymbol points between matched code and a local or artifact-qualified dependency declaration.
// Confidence is "exact" for a qualified identity match, "import-resolved" when an
// explicit import identifies the target module, "context-resolved" when declaration
// kind, file context, or receiver type disambiguates it, "unique-terminal" when only
// one declaration has the terminal name, "dependency-resolved" for one exact
// versioned artifact declaration, "dependency-candidate" for multiple exact-version
// declarations, and "dependency-unresolved" when versioned evidence has no artifact target.
type RelatedSymbol struct {
	Name           string                       `json:"name"`
	Path           string                       `json:"path"`
	Kind           string                       `json:"kind"`
	Direction      string                       `json:"direction"`
	Start          int                          `json:"start"`
	End            int                          `json:"end"`
	CallLine       int                          `json:"callLine"`
	Confidence     string                       `json:"confidence"`
	Role           string                       `json:"role,omitempty"`
	External       *ExternalNavigationReference `json:"external,omitempty"`
	Artifact       *NavigationArtifactIdentity  `json:"artifact,omitempty"`
	Segments       []ResultSegment              `json:"segments,omitempty"`
	Related        []RelatedSymbol              `json:"related,omitempty"`
	OmittedCallers int                          `json:"omittedCallers,omitempty"`
	OmittedCallees int                          `json:"omittedCallees,omitempty"`
	OmittedTypes   int                          `json:"omittedTypes,omitempty"`
}

// FileResult is one matching file.
type FileResult struct {
	Path                  string          `json:"path"`
	Repo                  string          `json:"repo,omitempty"`
	Language              string          `json:"language"`
	StructureStatus       string          `json:"structureStatus,omitempty"`
	Matches               []ResultMatch   `json:"matches"`
	Segments              []ResultSegment `json:"segments"`
	Context               []ContextLine   `json:"context,omitempty"`
	Related               []RelatedSymbol `json:"related,omitempty"`
	OmittedRelatedCallers int             `json:"omittedRelatedCallers,omitempty"`
	OmittedRelatedCallees int             `json:"omittedRelatedCallees,omitempty"`
	OmittedRelatedTypes   int             `json:"omittedRelatedTypes,omitempty"`
}

// RepoCount is a per-repository tally of matching files and lines.
type RepoCount struct {
	Repo    string `json:"repo"`
	Files   int    `json:"files"`
	Matches int    `json:"matches"`
}

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
	// RuleEngineText identifies legacy literal or regular-expression rules. The empty engine is also text.
	RuleEngineText = "text"
	// RuleEngineGritQL identifies native structural rules.
	RuleEngineGritQL = "gritql"
)

// Rule is a saved search materialized per repository.
type Rule struct {
	ID         string        `json:"id"`
	Name       string        `json:"name,omitempty"`
	Mode       string        `json:"mode"`
	Engine     string        `json:"engine,omitempty"`
	Request    SearchRequest `json:"request"`
	Structural *GritRequest  `json:"structural,omitempty"`
	CreatedAt  string        `json:"createdAt,omitempty"`
	UpdatedAt  string        `json:"updatedAt,omitempty"`
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

// RuleSet is a generation-stamped collection returned by the rules API.
type RuleSet struct {
	Generation int64  `json:"generation"`
	Rules      []Rule `json:"rules"`
}
