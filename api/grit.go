package api

import "github.com/greppleai/grepple/rulespec"

const (
	// GritCompatibilityV1 is the unified native structural-search contract.
	GritCompatibilityV1 = rulespec.GritCompatibilityV1
	// GritMultilingualCompatibilityV1 is retained as a source-compatible alias.
	// Deprecated: use GritCompatibilityV1.
	GritMultilingualCompatibilityV1 = GritCompatibilityV1
	// MaxGritRequestBodyBytes bounds one encoded structural-search request.
	MaxGritRequestBodyBytes = 2 << 20
	// MaxGritQueryBytes bounds the UTF-8 query source carried by a request.
	MaxGritQueryBytes = rulespec.MaxGritQueryBytes
	// MaxGritPatternIDBytes bounds a caller-provided pattern identifier.
	MaxGritPatternIDBytes = rulespec.MaxGritPatternIDBytes
	// MaxGritMessageBytes bounds a caller-provided finding message.
	MaxGritMessageBytes = rulespec.MaxGritMessageBytes
	// MaxGritGlobs bounds the number of include and exclude globs combined.
	MaxGritGlobs = rulespec.MaxGritGlobs
	// MaxGritGlobBytes bounds one UTF-8 glob.
	MaxGritGlobBytes = rulespec.MaxGritGlobBytes
	// MaxGritRepositories bounds the number of include and exclude repositories combined.
	MaxGritRepositories = rulespec.MaxGritRepositories
	// MaxGritRepositoryBytes bounds one UTF-8 repository identifier.
	MaxGritRepositoryBytes = rulespec.MaxGritRepositoryBytes
	// MaxGritPageLimit bounds one public structural-search response page.
	MaxGritPageLimit = 100
)

const (
	// GritBindingNode identifies a single-node metavariable binding.
	GritBindingNode = "node"
	// GritBindingList identifies an element-aligned list metavariable binding.
	GritBindingList = "list"
)

// GritRequest and GritLimits retain wire-compatible names for Rules-owned models.
type GritRequest = rulespec.StructuralRequest
type GritLimits = rulespec.StructuralLimits

// GritPosition is a one-based Unicode-scalar source position.
type GritPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// GritRange is a half-open byte and source-position range. Byte offsets are
// zero-based; positions are one-based Unicode-scalar coordinates.
type GritRange struct {
	StartByte int          `json:"startByte"`
	EndByte   int          `json:"endByte"`
	Start     GritPosition `json:"start"`
	End       GritPosition `json:"end"`
}

// GritStructuralNode is one normalized structural binding value. Named nodes
// use NodeKind and Children; token leaves use TokenKind and Lexeme.
type GritStructuralNode struct {
	NodeKind  string               `json:"nodeKind,omitempty"`
	TokenKind string               `json:"tokenKind,omitempty"`
	Lexeme    string               `json:"lexeme,omitempty"`
	Children  []GritStructuralNode `json:"children,omitempty"`
}

// GritBinding is one name-sorted metavariable binding. Ranges and Structural
// remain element-aligned for list bindings.
type GritBinding struct {
	Name       string               `json:"name"`
	Kind       string               `json:"kind"`
	Range      GritRange            `json:"range"`
	Ranges     []GritRange          `json:"ranges,omitempty"`
	Structural []GritStructuralNode `json:"structural"`
}

// GritFinding is one deterministic structural match qualified by repository
// and repository-relative path.
type GritFinding struct {
	Repo      string        `json:"repo,omitempty"`
	Path      string        `json:"path"`
	Language  string        `json:"language"`
	Range     GritRange     `json:"range"`
	Text      string        `json:"text"`
	PatternID string        `json:"patternId"`
	Message   string        `json:"message"`
	Bindings  []GritBinding `json:"bindings"`
}

// GritDiagnostic is one stable structural compile, source, resource, or
// cancellation diagnostic. Optional context fields encode as JSON null.
type GritDiagnostic struct {
	Code      string     `json:"code"`
	Class     string     `json:"class"`
	Severity  string     `json:"severity"`
	Message   string     `json:"message"`
	Repo      string     `json:"repo,omitempty"`
	Shard     string     `json:"shard,omitempty"`
	PatternID *string    `json:"patternId"`
	Path      *string    `json:"path"`
	Range     *GritRange `json:"range"`
}

// GritTruncation records deterministic scanner or distributed-result omission
// separately from diagnostics.
type GritTruncation struct {
	Repo    string `json:"repo,omitempty"`
	Shard   string `json:"shard,omitempty"`
	Reason  string `json:"reason"`
	Limit   int64  `json:"limit"`
	Skipped int    `json:"skipped"`
}

// GritEffectiveLimits publishes the concrete limits used for an execution.
type GritEffectiveLimits struct {
	PatternBytes      int   `json:"patternBytes"`
	RegexBytes        int   `json:"regexBytes"`
	RegexInstructions int   `json:"regexInstructions"`
	ParseDepth        int   `json:"parseDepth"`
	SourceBytes       int   `json:"sourceBytes"`
	Candidates        int   `json:"candidates"`
	ASTSteps          int   `json:"astSteps"`
	Findings          int   `json:"findings"`
	FileTimeMillis    int64 `json:"fileTimeMillis"`
	BatchTimeMillis   int64 `json:"batchTimeMillis"`
	MemoryBytes       int64 `json:"memoryBytes"`
}

// GritMetadata identifies the compatibility and grammar contract used for a response.
type GritMetadata struct {
	Compatibility string              `json:"compatibility"`
	Language      string              `json:"language,omitempty"`
	Grammar       string              `json:"grammar,omitempty"`
	GoGrammar     string              `json:"goGrammar"`
	Limits        GritEffectiveLimits `json:"limits"`
}

// GritStatistics contains deterministic scanner counters.
type GritStatistics struct {
	Candidates      int   `json:"candidates"`
	Eligible        int   `json:"eligible"`
	Evaluated       int   `json:"evaluated"`
	BytesRead       int64 `json:"bytesRead"`
	SkippedLanguage int   `json:"skippedLanguage"`
	SkippedGlob     int   `json:"skippedGlob"`
	SkippedBinary   int   `json:"skippedBinary"`
	SkippedAnchor   int   `json:"skippedAnchor"`
}

// GritResponse is the dedicated local, shard, and router structural-search
// response. Producers must initialize collection fields to non-nil slices.
type GritResponse struct {
	Metadata       GritMetadata     `json:"metadata"`
	ResultMetadata *ResultMetadata  `json:"resultMetadata,omitempty"`
	Findings       []GritFinding    `json:"findings"`
	Diagnostics    []GritDiagnostic `json:"diagnostics"`
	Truncations    []GritTruncation `json:"truncations"`
	Statistics     GritStatistics   `json:"statistics"`
	ShardErrors    []string         `json:"shardErrors"`
	Total          int              `json:"total"`
}
