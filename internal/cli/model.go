package cli

import "github.com/greppleai/grepple/search"

type cliOptions struct {
	Params           search.Params
	LineOnly         bool
	OnlyMatching     bool
	JSON             string
	Count            bool
	CountByRepo      bool
	FilesWithMatches bool
	Outline          bool
	Depth            int
	MaxOutputBytes   int
	Anchors          bool
	AnchorsDefaulted bool
	AnchorProvider   string
	AnchorLines      anchorLookup
	// Stdin is true when the search reads piped standard input instead of
	// walking the filesystem (see stdinSearch).
	Stdin bool
}
