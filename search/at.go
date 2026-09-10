package search

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	structure "github.com/greppleai/grepple/parser"
)

// At retrieves structural context for one local path and 1-based source line.
// Callable declarations are returned exactly for every structurally supported language.
func At(params Params) (*FileMatch, error) {
	path, line, err := parseAtReference(params.At)
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !withinRoot(absolute, params.Root) {
		return nil, fmt.Errorf("--at path %q is outside the search root", path)
	}
	contentBytes, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read --at path %q: %w", path, err)
	}
	if bytes.IndexByte(contentBytes, 0) >= 0 {
		return nil, fmt.Errorf("--at path %q is binary", path)
	}
	content := strings.ToValidUTF8(string(contentBytes), "\uFFFD")
	lines := SplitLines(content)
	if line < 1 || line > len(lines) {
		return nil, fmt.Errorf("--at line %d is outside %s (1-%d)", line, path, len(lines))
	}
	language := structure.LanguageFor(path)
	match := &FileMatch{
		File: absolute, DisplayPath: displayPath(absolute), Content: content, Language: language,
		MatchLines: map[int]bool{line: true}, SegmentsReady: true,
	}
	if start, end, ok := structure.DeclarationRangeAt(content, language, line); ok {
		match.Segments = []structure.Segment{{Kind: "lines", Start: start, End: end}}
	} else {
		match.Segments = structure.BuildSegments(content, language, match.MatchLines, params.MaxSegments)
	}
	if params.Related {
		files, collectErr := collectCandidateFiles(nil, params.Root)
		if collectErr != nil {
			return nil, collectErr
		}
		files = appendFileIfMissing(files, absolute)
		scan := candidateScan{p: params, repoFilter: NewRepoFilter(params.Repo, params.ExcludeRepo)}
		// Navigation attachment receives values, so copy its result back.
		attached := []FileMatch{*match}
		attachRelated(attached, scan.relatedFiles(files), params.FollowRelated)
		*match = attached[0]
	}
	return match, nil
}

func parseAtReference(reference string) (string, int, error) {
	separator := strings.LastIndex(reference, ":")
	if separator <= 0 || separator == len(reference)-1 {
		return "", 0, fmt.Errorf("--at requires PATH:LINE or PATH:START-END")
	}
	lineText := reference[separator+1:]
	if rangeSeparator := strings.IndexByte(lineText, '-'); rangeSeparator >= 0 {
		lineText = lineText[:rangeSeparator]
	}
	line, err := strconv.Atoi(lineText)
	if err != nil || line < 1 {
		return "", 0, fmt.Errorf("--at requires a positive line in PATH:LINE")
	}
	return reference[:separator], line, nil
}

func appendFileIfMissing(files []string, path string) []string {
	for _, file := range files {
		if filepath.Clean(file) == filepath.Clean(path) {
			return files
		}
	}
	return append(files, path)
}
