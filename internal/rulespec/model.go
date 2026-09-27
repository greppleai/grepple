package rulespec

import "github.com/greppleai/grepple/internal/search"

const (
	ModeCount    = "count"
	ModeFiles    = "files"
	EngineText   = "text"
	EngineGritQL = "gritql"

	GritCompatibilityV1    = "gritql-v1"
	MaxGritQueryBytes      = 1 << 20
	MaxGritPatternIDBytes  = 256
	MaxGritMessageBytes    = 16 << 10
	MaxGritGlobs           = 256
	MaxGritGlobBytes       = 4 << 10
	MaxGritRepositories    = 1024
	MaxGritRepositoryBytes = 1024
)

// StructuralRequest is the normalized structural-search input embedded in a rule.
type StructuralRequest struct {
	Query               string            `json:"query"`
	Compatibility       string            `json:"compatibility"`
	PatternID           string            `json:"patternId,omitempty"`
	Message             string            `json:"message,omitempty"`
	Globs               []string          `json:"globs,omitempty"`
	ExcludeGlobs        []string          `json:"excludeGlobs,omitempty"`
	Repositories        []string          `json:"repositories,omitempty"`
	ExcludeRepositories []string          `json:"excludeRepositories,omitempty"`
	Skip                *int              `json:"skip,omitempty"`
	Limit               *int              `json:"limit,omitempty"`
	Limits              *StructuralLimits `json:"limits,omitempty"`
}

// StructuralLimits carries optional compiler, evaluator, and scanner limits.
type StructuralLimits struct {
	PatternBytes      *int   `json:"patternBytes,omitempty"`
	RegexBytes        *int   `json:"regexBytes,omitempty"`
	RegexInstructions *int   `json:"regexInstructions,omitempty"`
	ParseDepth        *int   `json:"parseDepth,omitempty"`
	SourceBytes       *int   `json:"sourceBytes,omitempty"`
	Candidates        *int   `json:"candidates,omitempty"`
	ASTSteps          *int   `json:"astSteps,omitempty"`
	Findings          *int   `json:"findings,omitempty"`
	FileTimeMillis    *int64 `json:"fileTimeMillis,omitempty"`
	BatchTimeMillis   *int64 `json:"batchTimeMillis,omitempty"`
	MemoryBytes       *int64 `json:"memoryBytes,omitempty"`
	Files             *int   `json:"files,omitempty"`
	TotalBytes        *int64 `json:"totalBytes,omitempty"`
	Workers           *int   `json:"workers,omitempty"`
}

// Rule is a saved text or structural query normalized for materialization.
type Rule struct {
	ID         string             `json:"id"`
	Name       string             `json:"name,omitempty"`
	Mode       string             `json:"mode"`
	Engine     string             `json:"engine,omitempty"`
	Request    search.Request     `json:"request"`
	Structural *StructuralRequest `json:"structural,omitempty"`
	CreatedAt  string             `json:"createdAt,omitempty"`
	UpdatedAt  string             `json:"updatedAt,omitempty"`
}
