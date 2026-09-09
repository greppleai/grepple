package grepplecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"

	"grepple/internal/api"
	"grepple/internal/parser"
	"grepple/internal/search"
)

// ErrBrokenPipe is the sentinel returned by output writers when the consumer's
// pipe closes early (e.g. `grepple ... | head`); the CLI maps it to a clean exit.
var ErrBrokenPipe = errBrokenPipe{}

type errBrokenPipe struct{}

func (errBrokenPipe) Error() string { return "broken pipe" }

// Unwrap lets the shared command boundary recognize an early-closed pipe
// without depending on the CLI package.
func (errBrokenPipe) Unwrap() error { return syscall.EPIPE }

// SafeWrite writes s to stdout, translating EPIPE (consumer closed the pipe,
// e.g. `grepple ... | head`) into ErrBrokenPipe so callers can exit cleanly
// instead of reporting a spurious write failure.
func SafeWrite(s string) error {
	_, err := os.Stdout.WriteString(s)
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			return ErrBrokenPipe
		}
		return err
	}
	return nil
}

// Collapsed renders the "… N lines collapsed …" marker printed between
// segments (empty string when nothing was skipped).
func Collapsed(n int) string {
	if n <= 0 {
		return ""
	}
	word := "lines"
	if n == 1 {
		word = "line"
	}
	return fmt.Sprintf("\n// … %d %s collapsed …\n\n", n, word)
}

// PrintSegments renders a file's structural segments with line numbers,
// replacing the gaps between segments with collapsed-line markers. It writes
// only what the segments cover, so a bounded result page stays small.
func PrintSegments(content string, segs []parser.Segment) error {
	lines := search.SplitLines(content)
	width := len(fmt.Sprint(len(lines)))
	cursor := 1
	for _, segment := range segs {
		var err error
		if cursor, err = renderSegmentLines(lines, width, cursor, segment); err != nil {
			return err
		}
	}
	if cursor <= len(lines) {
		return SafeWrite(Collapsed(len(lines) - cursor + 1))
	}
	return nil
}

func renderSegmentLines(lines []string, width, cursor int, segment parser.Segment) (int, error) {
	if segment.Start > cursor {
		if err := SafeWrite(Collapsed(segment.Start - cursor)); err != nil {
			return cursor, err
		}
	}
	if segment.Kind == "summary" {
		if err := SafeWrite(fmt.Sprintf("%*d   %s\n", width, segment.Start, segment.Text)); err != nil {
			return cursor, err
		}
		return segment.End + 1, nil
	}
	for line := segment.Start; line <= segment.End; line++ {
		if err := SafeWrite(fmt.Sprintf("%*d   %s\n", width, line, lines[line-1])); err != nil {
			return cursor, err
		}
	}
	return segment.End + 1, nil
}

// JSONWrite renders value as indented JSON (HTML escaping off so regex
// metacharacters stay readable) and writes it to stdout via SafeWrite.
func JSONWrite(value any) error {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return SafeWrite(output.String())
}

// PrintCount renders one matching-line count per file. This intentionally follows
// grep/rg count semantics rather than the repository aggregation provided by
// PrintRepoCounts.
func PrintCount(results []api.FileResult, jsonMode bool) error {
	type fileCount struct {
		Path  string `json:"path"`
		Count int    `json:"count"`
	}
	counts := make([]fileCount, 0, len(results))
	for _, result := range results {
		counts = append(counts, fileCount{Path: result.Path, Count: len(result.Matches)})
	}
	if jsonMode {
		return JSONWrite(map[string]any{"counts": counts})
	}
	for _, count := range counts {
		if err := SafeWrite(fmt.Sprintf("%s\t%d\n", count.Path, count.Count)); err != nil {
			return err
		}
	}
	return nil
}

// PrintRepoCounts renders already-summed per-repository tallies plus a total, in
// deterministic narrowing order (hottest repositories first).
func PrintRepoCounts(counts []api.RepoCount, jsonMode bool) error {
	search.SortRepoCounts(counts)
	totalFiles, totalMatches := 0, 0
	for _, count := range counts {
		totalFiles += count.Files
		totalMatches += count.Matches
	}
	if jsonMode {
		repos := map[string]any{}
		for _, count := range counts {
			if count.Repo != "" {
				repos[count.Repo] = map[string]any{"files": count.Files, "matches": count.Matches}
			}
		}
		return JSONWrite(map[string]any{"count": map[string]any{"files": totalFiles, "matches": totalMatches, "repos": repos}})
	}
	printedRepo := false
	for _, count := range counts {
		if count.Repo == "" {
			continue
		}
		printedRepo = true
		if err := SafeWrite(fmt.Sprintf("%s\t%d files\t%d matches\n", count.Repo, count.Files, count.Matches)); err != nil {
			return err
		}
	}
	if printedRepo {
		return SafeWrite(fmt.Sprintf("total\t%d files\t%d matches\n", totalFiles, totalMatches))
	}
	return SafeWrite(fmt.Sprintf("%d files\t%d matches\n", totalFiles, totalMatches))
}
