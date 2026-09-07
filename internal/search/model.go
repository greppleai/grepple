package search

import "grepple/internal/parser"

// DefaultMaxSegments caps how many structural segments one file result includes
// when the caller does not set MaxSegments.
const DefaultMaxSegments = 20

// Params is the fully resolved parameter set the engine executes: every
// default applied, no pointer fields.
type Params struct {
	Query        string   `json:"query"`
	Globs        []string `json:"globs"`
	Regex        bool     `json:"regex"`
	IgnoreCase   bool     `json:"ignoreCase"`
	MaxFiles     int      `json:"maxFiles,omitempty"`
	MaxSegments  int      `json:"maxSegments"`
	Skip         int      `json:"skip,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	Repo         []string `json:"repo,omitempty"`
	ExcludeRepo  []string `json:"excludeRepo,omitempty"`
	Files        bool     `json:"files,omitempty"`
	Context      int      `json:"context,omitempty"`
	SkipSegments bool     `json:"-"`
	Root         string   `json:"-"`
	CountByRepo  bool     `json:"-"`
}

// FileMatch is the engine's internal per-file match: content, matching lines,
// and parser-produced structural segments.
type FileMatch struct {
	File, DisplayPath, Content, Language string
	MatchLines                           map[int]bool
	Segments                             []parser.Segment
	SegmentsReady                        bool
}
