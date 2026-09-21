package cli

import (
	"github.com/greppleai/grepple/api"
	anchorscommand "github.com/greppleai/grepple/internal/cli/anchors"
	"github.com/greppleai/grepple/search"
)

type cliOptions struct {
	Params           search.Params
	LineOnly         bool
	OnlyMatching     bool
	JSON             string
	Count            bool
	CountByRepo      bool
	CountSummary     bool
	FilesWithMatches bool
	Outline          bool
	Depth            int
	MaxOutputBytes   int
	RepeatSource     bool
	Anchors          bool
	AnchorLines      anchorscommand.Lookup
	ResultMetadata   *api.ResultMetadata
	// Stdin is true when the search reads piped standard input instead of
	// walking the filesystem (see stdinSearch).
	Stdin bool
}
