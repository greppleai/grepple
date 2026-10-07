package search

import (
	"fmt"
	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func cursorSearchEligible(options *Options) bool {
	p := options.Params
	return !options.MixedRemote && p.At == "" && !p.Files && !options.FilesWithMatches && !options.Count && !options.CountSummary && !options.CountByRepo && !p.CountByRepo && p.Skip == 0 && p.MaxFiles == 0 && !p.ProductionOnly && (p.Sort == "" || p.Sort == search.ResultSortPath) && p.Query != ""
}
func validateCursorArguments(values *Args) error {
	if values.RemoteOnly && values.Outline {
		return fmt.Errorf("--outline is a local-only operation")
	}
	if values.RemoteOnly && values.Local {
		return fmt.Errorf("--remote-only cannot be combined with --local")
	}
	if values.Cursor == "" {
		return nil
	}
	if !apiclient.ValidSearchCursor(values.Cursor) {
		return fmt.Errorf("invalid --cursor token")
	}
	if values.Local || values.Skip != 0 || values.At != "" || values.Files || values.Count || values.CountByRepo || values.CountSummary || values.MaxFiles != 0 || values.Outline || values.Sort != search.ResultSortPath {
		return fmt.Errorf("--cursor is only supported for remote content search without offsets, counts, listing or match sorting")
	}
	return nil
}
func cursorResultMetadata(application cliruntime.Context, options *Options, metadata *wire.ResultMetadata) {
	page := options.CursorPage
	if page == nil {
		return
	}
	metadata.Scope.Mode = "remote"
	metadata.Order = "snapshot-traversal"
	metadata.Page.Total = nil
	metadata.Page.Complete = page.Complete && metadata.Page.Complete
	metadata.Omitted.Files = 0
	metadata.NextCommand = ""
	if !page.Complete {
		metadata.NextCommand = searchNextCommand(application, options, 0, true)
	}
}
