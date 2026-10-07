package search

import (
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/wire"
)

// A frozen page is indivisible: reducing --limit on replay must not discard files.
func selectedSearchWindow(options *Options, results []wire.FileResult) []wire.FileResult {
	if options.CursorPage == nil {
		return windowResults(results, options.Params)
	}
	options.Params.Limit = max(options.Params.Limit, len(results))
	return results
}
func finishSearchExit(application cliruntime.Context, options *Options, results []wire.FileResult) error {
	if options.CursorPage != nil && !options.CursorPage.Complete && len(results) == 0 {
		return nil
	}
	return setSearchExit(application, results)
}
