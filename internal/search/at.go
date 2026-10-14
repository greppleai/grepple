package search

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/linerange"
	structure "github.com/greppleai/grepple/internal/parser"
)

// At retrieves local structural context by legacy PATH:LINE[-END] or a typed,
// current-file verified scope selector. Legacy callable navigation is unchanged.
func At(params Params) (*FileMatch, error) {
	path, line, endLine, err := atReference(params)
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
	language := structure.NewParser().LanguageFor(path)
	var resolved linerange.Result
	if params.AtScope != nil {
		line, endLine, err = verifiedAtScope(params, path, content, structure.ScopeContentLanguageFor(path), nil, len(lines))
		if err != nil {
			return nil, err
		}
	} else {
		resolved, err = linerange.Resolve(line, endLine, len(lines))
		if err != nil {
			return nil, fmt.Errorf("--at %s: %w", path, err)
		}
		endLine = resolved.ReturnedEnd
	}
	match := &FileMatch{
		File: absolute, DisplayPath: displayPathFrom(absolute, displayBase(params.Root)), Content: content, Language: language,
		MatchLines: map[int]bool{line: true}, SegmentsReady: true,
	}
	if params.AtScope == nil && resolved.Outcome != linerange.OutcomeExact {
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
	path, line, endLine, err := atReference(params)
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
	language := document.Language()
	var resolved linerange.Result
	if params.AtScope != nil {
		if language != structure.ScopeContentLanguageFor(path) {
			return nil, fmt.Errorf("--at selector document language %q disagrees with path %q", language, path)
		}
		line, endLine, err = verifiedAtScope(params, path, content, language, document, len(lines))
		if err != nil {
			return nil, err
		}
	} else {
		resolved, err = linerange.Resolve(line, endLine, len(lines))
		if err != nil {
			return nil, fmt.Errorf("--at %s: %w", path, err)
		}
		endLine = resolved.ReturnedEnd
	}
	match := &FileMatch{
		File: absolute, DisplayPath: displayPathFrom(absolute, displayBase(params.Root)), Content: content, Language: language,
		MatchLines: map[int]bool{line: true}, SegmentsReady: true,
	}
	if params.AtScope == nil && resolved.Outcome != linerange.OutcomeExact {
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
	if params.AtScope != nil || endLine > line {
		match.Segments = []structure.Segment{{Kind: "lines", Start: line, End: endLine}}
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
			match.Segments, match.StructureStatus = structure.NewParser().Segments(document, match.MatchLines)
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

// atReference keeps the old PATH:LINE grammar independent of local scope
// requests, which carry the path and selector in separate typed fields.
func atReference(params Params) (string, int, int, error) {
	if params.AtScope == nil {
		return parseAtRange(params.At)
	}
	s := params.AtScope
	if params.At == "" {
		return "", 0, 0, fmt.Errorf("--at scope selector requires a path")
	}
	if !structure.PublicScopeKind(s.Kind) {
		return "", 0, 0, fmt.Errorf("--at selector has unknown scope kind %q", s.Kind)
	}
	if s.StartLine < 1 || s.EndLine < s.StartLine || !s.Exact && s.StartLine != s.EndLine {
		return "", 0, 0, fmt.Errorf("--at selector requires a positive ascending @KIND:LINE or @KIND:START-END")
	}
	return params.At, s.StartLine, s.EndLine, nil
}

func verifiedAtScope(params Params, path, content, language string, document *structure.Document, lineCount int) (int, int, error) {
	s := *params.AtScope
	if s.EndLine > lineCount {
		return 0, 0, fmt.Errorf("--at selector %s:%d-%d exceeds %s (%d lines); scope bounds may be stale", s.Kind, s.StartLine, s.EndLine, path, lineCount)
	}
	if language == "text" {
		return 0, 0, fmt.Errorf("--at selector unsupported for plain or unclassified source %s", path)
	}
	span, outcome := structure.ResolveScopeSelector(content, language, s, document)
	switch outcome {
	case structure.ScopeVerified:
		return span.StartLine, span.EndLine, nil
	case structure.ScopeAmbiguous:
		return 0, 0, fmt.Errorf("--at selector @%s:%d is ambiguous in %s; specify an exact @%s:START-END span", s.Kind, s.StartLine, path, s.Kind)
	default:
		return 0, 0, fmt.Errorf("--at selector @%s:%d-%d has no verified current scope in %s (unsupported syntax, missing scope or stale bounds)", s.Kind, s.StartLine, s.EndLine, path)
	}
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
