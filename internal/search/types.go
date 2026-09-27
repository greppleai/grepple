package search

import "github.com/greppleai/grepple/internal/navigation"

// Request is the unresolved search input shared by local and transport adapters.
// Nil pointer fields preserve the distinction between unset and explicit zero values.
type Request struct {
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

type ResultSegment = navigation.ResultSegment

// ContextLine is one line of match context.
type ContextLine struct {
	Line  int    `json:"line"`
	Text  string `json:"text"`
	Match bool   `json:"match"`
}

// LineRangeResult reports how an inclusive source range intersected a file.
type LineRangeResult struct {
	RequestedStart int    `json:"requestedStart"`
	RequestedEnd   int    `json:"requestedEnd"`
	ReturnedStart  int    `json:"returnedStart,omitempty"`
	ReturnedEnd    int    `json:"returnedEnd,omitempty"`
	FileLines      int    `json:"fileLines"`
	Outcome        string `json:"outcome"`
	Warning        string `json:"warning,omitempty"`
}

// FileResult is one matching file returned by the Search domain.
type FileResult struct {
	Path                  string                     `json:"path"`
	Repo                  string                     `json:"repo,omitempty"`
	Language              string                     `json:"language"`
	StructureStatus       string                     `json:"structureStatus,omitempty"`
	Matches               []ResultMatch              `json:"matches"`
	Segments              []ResultSegment            `json:"segments"`
	Context               []ContextLine              `json:"context,omitempty"`
	LineRange             *LineRangeResult           `json:"lineRange,omitempty"`
	Related               []navigation.RelatedSymbol `json:"related,omitempty"`
	OmittedRelatedCallers int                        `json:"omittedRelatedCallers,omitempty"`
	OmittedRelatedCallees int                        `json:"omittedRelatedCallees,omitempty"`
	OmittedRelatedTypes   int                        `json:"omittedRelatedTypes,omitempty"`
}

func (result FileResult) ExternalDependencyData() navigation.ExternalDependencyData {
	return navigation.ExternalDependencyData{Path: result.Path, Repo: result.Repo, Language: result.Language, Related: result.Related}
}

func (result FileResult) WithExternalDependencyRelated(related []navigation.RelatedSymbol) FileResult {
	result.Related = related
	return result
}

// RepoCount is a per-repository tally of matching files and lines.
type RepoCount struct {
	Repo    string `json:"repo"`
	Files   int    `json:"files"`
	Matches int    `json:"matches"`
}
