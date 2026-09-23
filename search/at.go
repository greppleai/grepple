package search

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/linerange"
	structure "github.com/greppleai/grepple/parser"
)

// At retrieves structural context for one local path and 1-based source line.
// Callable declarations are returned exactly for every structurally supported language.
func At(params Params) (*FileMatch, error) {
	path, line, endLine, err := parseAtRange(params.At)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(params.Root) != "" {
		path = filepath.Join(params.Root, path)
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
	resolved, err := linerange.Resolve(line, endLine, len(lines))
	if err != nil {
		return nil, fmt.Errorf("--at %s: %w", path, err)
	}
	endLine = resolved.ReturnedEnd
	language := structure.LanguageFor(path)
	match := &FileMatch{
		File: absolute, DisplayPath: displayPathFrom(absolute, displayBase(params.Root)), Content: content, Language: language,
		MatchLines: map[int]bool{line: true}, SegmentsReady: true,
	}
	if resolved.Outcome != linerange.OutcomeExact {
		match.LineRange = &resolved
	}
	if err := prepareAtMatch(match, params, line, endLine, nil); err != nil {
		return nil, err
	}
	if params.Related && match.CallableDeclaration {
		files, collectErr := collectCandidateFiles(nil, params.Root)
		if collectErr != nil {
			return nil, collectErr
		}
		files = appendFileIfMissing(files, absolute)
		scan := candidateScan{p: params, repoFilter: NewRepoFilter(params.Repo, params.ExcludeRepo)}
		// Navigation attachment receives values, so copy its result back.
		attached := []FileMatch{*match}
		attachRelatedAt(attached, scan.relatedFiles(files), params.FollowRelated)
		*match = attached[0]
	}
	return match, nil
}

// AtFromDocument retrieves local structural context from a caller-owned document
// and attaches related evidence from an already resolved navigation analysis.
func AtFromDocument(params Params, document *structure.Document, analysis *NavigationAnalysis) (*FileMatch, error) {
	path, line, endLine, err := parseAtRange(params.At)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(params.Root) != "" {
		path = filepath.Join(params.Root, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !withinRoot(absolute, params.Root) {
		return nil, fmt.Errorf("--at path %q is outside the search root", path)
	}
	if document == nil {
		return nil, fmt.Errorf("--at path %q has no parsed document", path)
	}
	content := document.Source()
	lines := SplitLines(content)
	resolved, err := linerange.Resolve(line, endLine, len(lines))
	if err != nil {
		return nil, fmt.Errorf("--at %s: %w", path, err)
	}
	endLine = resolved.ReturnedEnd
	language := document.Language()
	match := &FileMatch{
		File: absolute, DisplayPath: displayPathFrom(absolute, displayBase(params.Root)), Content: content, Language: language,
		MatchLines: map[int]bool{line: true}, SegmentsReady: true,
	}
	if resolved.Outcome != linerange.OutcomeExact {
		match.LineRange = &resolved
	}
	if err := prepareAtMatch(match, params, line, endLine, document); err != nil {
		return nil, err
	}
	if params.Related && match.CallableDeclaration {
		AttachRelatedFromAnalysis(match, analysis, params.FollowRelated)
	}
	return match, nil
}

func prepareAtMatch(match *FileMatch, params Params, line, endLine int, document *structure.Document) error {
	for selected := line; selected <= endLine; selected++ {
		match.MatchLines[selected] = true
	}
	if params.LineRanges || params.BeforeContext > 0 || params.AfterContext > 0 {
		return nil
	}
	if endLine > line {
		match.Segments = []structure.Segment{{Kind: "lines", Start: line, End: endLine}}
		return nil
	}
	if document != nil {
		if start, end, ok := structure.DeclarationRangeAtFromDocument(document, match.DisplayPath, line); ok {
			match.CallableDeclaration = true
			match.Segments = []structure.Segment{{Kind: "lines", Start: start, End: end}}
		} else {
			match.Segments, match.StructureStatus = structure.BuildSegmentsFromDocument(document, match.MatchLines)
		}
		return nil
	}
	if start, end, ok := structure.DeclarationRangeAt(match.Content, match.Language, line); ok {
		match.CallableDeclaration = true
		match.Segments = []structure.Segment{{Kind: "lines", Start: start, End: end}}
	} else {
		match.Segments = structure.BuildSegments(match.Content, match.Language, match.MatchLines)
	}
	return nil
}

func parseAtReference(reference string) (string, int, error) {
	path, start, _, err := parseAtRange(reference)
	return path, start, err
}

func parseAtRange(reference string) (string, int, int, error) {
	separator := strings.LastIndex(reference, ":")
	if separator <= 0 || separator == len(reference)-1 {
		return "", 0, 0, fmt.Errorf("--at requires PATH:LINE or PATH:START-END")
	}
	lineText := reference[separator+1:]
	startText, endText := lineText, lineText
	if rangeSeparator := strings.IndexByte(lineText, '-'); rangeSeparator >= 0 {
		startText, endText = lineText[:rangeSeparator], lineText[rangeSeparator+1:]
	}
	start, startErr := strconv.Atoi(startText)
	end, endErr := strconv.Atoi(endText)
	if startErr != nil || endErr != nil || start < 1 || end < start {
		return "", 0, 0, fmt.Errorf("--at requires a positive ascending range in PATH:LINE or PATH:START-END")
	}
	return reference[:separator], start, end, nil
}

func appendFileIfMissing(files []string, path string) []string {
	for _, file := range files {
		if filepath.Clean(file) == filepath.Clean(path) {
			return files
		}
	}
	return append(files, path)
}
