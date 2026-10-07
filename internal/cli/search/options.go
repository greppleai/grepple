package search

import (
	"github.com/greppleai/grepple/internal/anchor"
	"github.com/greppleai/grepple/internal/apiclient"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

type Options struct {
	Params           search.Params
	RemoteOnly       bool
	Cursor           string
	CursorPage       *apiclient.SearchPage
	MixedRemote      bool
	RemoteServer     string
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
