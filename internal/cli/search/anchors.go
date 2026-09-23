package search

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/anchor"
	searchengine "github.com/greppleai/grepple/search"
)

type anchorFileSelection struct {
	displayPath string
	lines       map[int]string
}

func prepareAnchors(options *Options, results []api.FileResult) (anchor.Lookup, error) {
	if !options.Anchors {
		return nil, nil
	}
	selections := collectAnchorSelections(options, results)
	files := make([]anchor.File, 0, len(selections))
	for _, selection := range selections {
		file, err := anchorFile(selection)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return anchor.Generate(files)
}

func collectAnchorSelections(options *Options, results []api.FileResult) []anchorFileSelection {
	byPath := make(map[string]map[int]string)
	for _, result := range results {
		lines := anchorSelectionLines(byPath, result.Path)
		collectAnchorSelectionLines(options, result, lines)
		collectRelatedTypeAnchorLines(result.Related, byPath)
	}
	paths := make([]string, 0, len(byPath))
	for path, lines := range byPath {
		if len(lines) > 0 {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	selections := make([]anchorFileSelection, 0, len(paths))
	for _, path := range paths {
		selections = append(selections, anchorFileSelection{displayPath: path, lines: byPath[path]})
	}
	return selections
}

func anchorSelectionLines(byPath map[string]map[int]string, path string) map[int]string {
	if byPath[path] == nil {
		byPath[path] = make(map[int]string)
	}
	return byPath[path]
}

func collectRelatedTypeAnchorLines(points []api.RelatedSymbol, byPath map[string]map[int]string) {
	for _, point := range points {
		if point.Direction == "type" && point.Path != "" && point.Artifact == nil {
			lines := anchorSelectionLines(byPath, point.Path)
			for _, segment := range point.Segments {
				collectAnchorSegmentLines(lines, segment)
			}
		}
		collectRelatedTypeAnchorLines(point.Related, byPath)
	}
}

func collectAnchorSelectionLines(options *Options, result api.FileResult, lines map[int]string) {
	if options.Params.BeforeContext > 0 || options.Params.AfterContext > 0 {
		for _, line := range result.Context {
			lines[line.Line] = normalizeRenderedAnchorLine(line.Text)
		}
		return
	}
	if options.LineOnly {
		for _, match := range result.Matches {
			lines[match.Line] = normalizeRenderedAnchorLine(match.Text)
		}
		return
	}
	for _, segment := range result.Segments {
		collectAnchorSegmentLines(lines, segment)
	}
}

func collectAnchorSegmentLines(lines map[int]string, segment api.ResultSegment) {
	if segment.Kind != "lines" {
		return
	}
	for index, line := range strings.Split(segment.Text, "\n") {
		lines[segment.Start+index] = normalizeRenderedAnchorLine(line)
	}
}

func anchorFile(selection anchorFileSelection) (anchor.File, error) {
	absolutePath, err := filepath.Abs(selection.displayPath)
	if err != nil {
		return anchor.File{}, fmt.Errorf("resolve anchor path %s: %w", selection.displayPath, err)
	}
	contentBytes, err := os.ReadFile(absolutePath)
	if err != nil {
		return anchor.File{}, fmt.Errorf("read anchor source %s: %w", selection.displayPath, err)
	}
	content, err := anchor.NormalizeContent(string(contentBytes))
	if err != nil {
		return anchor.File{}, fmt.Errorf("anchor source %s: %w", selection.displayPath, err)
	}
	fileLines := searchengine.SplitLines(content)
	lineNumbers := sortedAnchorLines(selection.lines)
	for _, line := range lineNumbers {
		if line < 1 || line > len(fileLines) || fileLines[line-1] != selection.lines[line] {
			return anchor.File{}, fmt.Errorf("anchor source %s changed after search; rerun Grepple", selection.displayPath)
		}
	}
	return anchor.File{Path: absolutePath, DisplayPath: selection.displayPath, Content: content, Lines: lineNumbers}, nil
}

func normalizeRenderedAnchorLine(line string) string { return strings.TrimSuffix(line, "\r") }

func sortedAnchorLines(lines map[int]string) []int {
	result := make([]int, 0, len(lines))
	for line := range lines {
		result = append(result, line)
	}
	sort.Ints(result)
	return result
}
