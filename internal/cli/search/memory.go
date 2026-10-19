package search

import (
	"path/filepath"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/memory"
	"github.com/greppleai/grepple/internal/wire"
)

// selectedMemoryReferences runs strictly after search result windowing. The
// scope checks are deliberately redundant: a remote page must never be
// interpreted as a repository-local source merely because paths coincide.
func selectedMemoryReferences(app cliruntime.Context, options *Options, explicitServer string, remote bool, results []wire.FileResult) *memory.Discovery {
	if remote || explicitServer != "" || options.RemoteOnly || options.MixedRemote || options.Cursor != "" || options.CursorPage != nil ||
		options.Stdin || options.Count || options.CountByRepo || options.CountSummary || options.FilesWithMatches || options.Outline ||
		options.RelatedExplicit || options.Params.At != "" || options.Params.Files || len(options.Params.Repo) > 0 || len(results) == 0 {
		return nil
	}
	root, err := memory.Root(app.Repository().WorkingDirectory())
	if err != nil {
		return nil
	}
	cwd := app.Repository().WorkingDirectory()
	page := make([]memory.MatchedFile, 0, len(results))
	for _, result := range results {
		if result.Repo != "" || len(result.Matches) == 0 {
			continue
		}
		target := result.Path
		if !filepath.IsAbs(target) {
			target = filepath.Join(cwd, filepath.FromSlash(target))
		}
		rel, err := filepath.Rel(root, filepath.Clean(target))
		if err != nil {
			continue
		}
		normalized := filepath.ToSlash(rel)
		if !memory.ValidPath(normalized) {
			continue
		}
		item := memory.MatchedFile{Path: normalized}
		for _, match := range result.Matches {
			if match.Line > 0 {
				item.Lines = append(item.Lines, match.Line)
			}
			// StartLine/EndLine in the search wire model are STRUCTURAL ranges,
			// not multiline query evidence. Never expand them into match spans.
		}
		if len(item.Lines) > 0 {
			page = append(page, item)
		}
	}
	if len(page) == 0 {
		return nil
	}
	found := memory.Discover(root, page)
	if found.Complete && len(found.References) == 0 {
		return nil
	}
	return &found
}
