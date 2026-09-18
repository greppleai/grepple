package search

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
)

const (
	// ResultSortPath preserves deterministic repository/path order.
	ResultSortPath = "path"
	// ResultSortMatches ranks matching-line count descending, then path ascending.
	ResultSortMatches = "matches"
)

// Params is the fully resolved parameter set the engine executes: every
// default applied, no pointer fields.
type Params struct {
	Query           string   `json:"query"`
	Globs           []string `json:"globs"`
	Regex           bool     `json:"regex"`
	IgnoreCase      bool     `json:"ignoreCase"`
	InvertMatch     bool     `json:"invertMatch,omitempty"`
	MaxFiles        int      `json:"maxFiles,omitempty"`
	Skip            int      `json:"skip,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Sort            string   `json:"sort,omitempty"`
	Repo            []string `json:"repo,omitempty"`
	ExcludeRepo     []string `json:"excludeRepo,omitempty"`
	Files           bool     `json:"files,omitempty"`
	Context         int      `json:"context,omitempty"`
	BeforeContext   int      `json:"beforeContext,omitempty"`
	AfterContext    int      `json:"afterContext,omitempty"`
	SkipSegments    bool     `json:"-"`
	LineRanges      bool     `json:"matchLineRanges,omitempty"`
	EnclosingRanges bool     `json:"enclosingLineRanges,omitempty"`
	Related         bool     `json:"related,omitempty"`
	FollowRelated   int      `json:"followRelated,omitempty"`
	NoRelated       bool     `json:"-"`
	At              string   `json:"-"`
	Root            string   `json:"-"`
	IgnorePaths     []string `json:"-"`
	IgnoreRoot      string   `json:"-"`
	ProductionOnly  bool     `json:"-"`
	CountByRepo     bool     `json:"-"`
}

// RelatedPreview is an optionally expanded declaration and its next navigation
// points. Content stays internal and is converted to wire segments at the boundary.
type RelatedPreview struct {
	Content                                      string
	Start, End                                   int
	Related                                      []RelatedPoint
	OmittedCallers, OmittedCallees, OmittedTypes int
}

// RelatedPoint is an internal navigation hint between matched source declarations.
type RelatedPoint struct {
	Name, Path, File, Kind, Direction, Confidence, Role string
	Start, End, CallLine, Distance                      int
	External                                            *api.ExternalNavigationReference
	Preview                                             *RelatedPreview
}

// FileMatch is the engine's internal per-file match: content, matching lines,
// parser-produced structural segments, and optional related declarations.
type FileMatch struct {
	File, DisplayPath, Content, Language                              string
	MatchLines                                                        map[int]bool
	MatchRanges                                                       map[int]parser.StructuralLineRange
	Segments                                                          []parser.Segment
	Related                                                           []RelatedPoint
	OmittedRelatedCallers, OmittedRelatedCallees, OmittedRelatedTypes int
	SegmentsReady                                                     bool
	CallableDeclaration                                               bool
	StructureStatus                                                   parser.SegmentBuildStatus
}
