package grepplecli

import "grepple/internal/search"

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
	// Stdin is true when the search reads piped standard input instead of
	// walking the filesystem (see stdinSearch).
	Stdin bool
}
