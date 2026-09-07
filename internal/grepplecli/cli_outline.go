package grepplecli

import (
	"bytes"
	"fmt"
	"os"

	"grepple/internal/parser"
	"grepple/internal/search"
)

// runOutline enumerates the local files matching the given globs and prints each
// file's structural outline (classes, functions, interfaces, ...) instead of
// searching their contents. It is the engine behind `grepple --outline`.
func runOutline(options *cliOptions) error {
	paths, err := search.ListFilePaths(options.Params, nil)
	if err != nil {
		return err
	}
	jsonMode := options.JSON != "off"
	// Mirror search's default-cap hint so a broad glob doesn't silently truncate.
	if options.Params.Limit == DefaultResultLimit && options.Params.MaxFiles == 0 && len(paths) == DefaultResultLimit {
		fmt.Fprintf(os.Stderr, "note: outlining the first %d files (default --limit); pass --limit N or --limit 0 for all\n", DefaultResultLimit)
	}

	outlines, printedAny, err := outlineFiles(options.Depth, paths, jsonMode)
	if err != nil {
		return err
	}
	if jsonMode {
		return JSONWrite(map[string]any{"files": outlines})
	}
	if !printedAny {
		setExit(1)
	}
	return nil
}

// outlineFiles walks the matched files, collecting every outline in JSON mode
// or printing non-empty ones (blank-line separated) in human mode.
func outlineFiles(depth int, paths []string, jsonMode bool) ([]parser.FileOutline, bool, error) {
	outlines := make([]parser.FileOutline, 0, len(paths))
	printedAny := false
	for _, path := range paths {
		outline, data, ok := loadFileOutline(path, depth)
		if !ok {
			continue
		}
		if jsonMode {
			outlines = append(outlines, outline)
			continue
		}
		if len(outline.Symbols) == 0 {
			continue // no definitions to show in human mode
		}
		if printedAny {
			if err := SafeWrite("\n"); err != nil {
				return nil, false, err
			}
		}
		if err := SafeWrite(RenderOutlineOrContent(outline, string(data))); err != nil {
			return nil, false, err
		}
		printedAny = true
	}
	return outlines, printedAny, nil
}

// loadFileOutline reads one file and outlines it; ok=false for unreadable
// (e.g. a race with deletion) or binary files, which are skipped silently.
func loadFileOutline(path string, depth int) (parser.FileOutline, []byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return parser.FileOutline{}, nil, false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return parser.FileOutline{}, nil, false // binary file
	}
	return parser.OutlineFileDepth(path, string(data), depth), data, true
}
