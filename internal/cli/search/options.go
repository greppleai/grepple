package search

import (
	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/anchor"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/search"
)

type Options struct {
	Params           search.Params
	LineOnly         bool
	OnlyMatching     bool
	JSON             string
	Count            bool
	CountByRepo      bool
	CountSummary     bool
	FilesWithMatches bool
	Outline          bool
	Kinds            rendercommand.OutlineKinds
	Depth            int
	MaxOutputBytes   int
	RepeatSource     bool
	Anchors          bool
	AnchorLines      anchor.Lookup
	ResultMetadata   *wire.ResultMetadata
	// Stdin is true when the search reads piped standard input instead of
	// walking the filesystem (see stdinSearch).
	Stdin bool
}
