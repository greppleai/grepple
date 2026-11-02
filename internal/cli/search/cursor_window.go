package search

import (
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/wire"
)

// Server-paged results must not have their offset applied a second time.
func selectedSearchWindow(options *Options, results []wire.FileResult) []wire.FileResult {
	if options.CursorPage != nil {
		options.Params.Limit = max(options.Params.Limit, len(results))
		return results
	}
	params := options.Params
	if options.ServerWindow {
		params.Skip = 0
	}
	return windowResults(results, params)
}
func finishSearchExit(application cliruntime.Context, options *Options, results []wire.FileResult) error {
	if options.CursorPage != nil && !options.CursorPage.Complete && len(results) == 0 {
		return nil
	}
	return setSearchExit(application, results)
}
