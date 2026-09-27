package render

import (
	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/search"
)

var activeInlineOutputThreshold int

type cliOptions struct {
	Params           search.Params
	LineOnly         bool
	OnlyMatching     bool
	JSON             string
	Count            bool
	FilesWithMatches bool
	MaxOutputBytes   int
	RepeatSource     bool
	AnchorLines      AnchorLookup
	ResultMetadata   *wire.ResultMetadata
	Stdin            bool
}

func contextGuardForResults(options *cliOptions, results []wire.FileResult) *segmentContextGuard {
	return newContextGuard(SearchOptions{
		Options:         Options{Params: options.Params, LineOnly: options.LineOnly, OnlyMatching: options.OnlyMatching, JSON: options.JSON, Count: options.Count, FilesWithMatches: options.FilesWithMatches, MaxOutputBytes: options.MaxOutputBytes, RepeatSource: options.RepeatSource, Anchors: options.AnchorLines, Metadata: options.ResultMetadata, Stdin: options.Stdin},
		ContextEnabled:  true,
		InlineThreshold: activeInlineOutputThreshold,
	}, results)
}

func invalidateRenderedContext(reason string) error { return InvalidateContext(reason) }
