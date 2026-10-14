package render

import (
	"fmt"
	"github.com/greppleai/grepple/internal/search"
	"regexp"
	"sort"
)

type lineRenderer struct {
	output  *outputWriter
	anchors AnchorLookup
}

func (renderer lineRenderer) Render(results []search.FileResult) error {
	if renderer.anchors != nil {
		return renderer.renderAnchored(results)
	}
	for _, result := range results {
		for _, match := range result.Matches {
			location := fmt.Sprintf("%d", match.Line)
			if match.StartLine > 0 && match.EndLine >= match.StartLine {
				location = fmt.Sprintf("%d@%d-%d", match.Line, match.StartLine, match.EndLine)
			} else if match.EndLine > match.Line {
				location = fmt.Sprintf("%d-%d", match.Line, match.EndLine)
			}
			if err := renderer.output.writeString(fmt.Sprintf("%s:%s:%s\n", result.Path, location, match.Text)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (renderer lineRenderer) renderAnchored(results []search.FileResult) error {
	for resultIndex, result := range results {
		if resultIndex > 0 {
			if err := renderer.output.writeString("\n"); err != nil {
				return err
			}
		}
		if err := renderer.output.writeString(result.Path + "\n\n"); err != nil {
			return err
		}
		if err := renderer.renderAnchoredResult(result); err != nil {
			return err
		}
	}
	return nil
}

func (renderer lineRenderer) renderAnchoredResult(result search.FileResult) error {
	matches := append([]search.ResultMatch(nil), result.Matches...)
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Line < matches[j].Line })
	for _, match := range matches {
		if err := renderer.renderAnchoredMatch(result, match); err != nil {
			return err
		}
	}
	return nil
}

func (renderer lineRenderer) renderAnchoredMatch(result search.FileResult, match search.ResultMatch) error {
	return renderer.output.writeString(renderer.anchoredMatchRow(result, match))
}

func (renderer lineRenderer) anchoredMatchRow(result search.FileResult, match search.ResultMatch) string {
	anchor := renderer.anchors.Line(result.Path, match.Line)
	return fmt.Sprintf("%s%s%d%s%s\n", anchor, anchorOutputSeparator, match.Line, anchorOutputSeparator, normalizeRenderedAnchorLine(match.Text))
}

type onlyMatchingRenderer struct {
	output  *outputWriter
	matcher *regexp.Regexp
}

func (renderer onlyMatchingRenderer) Render(results []search.FileResult) error {
	for _, result := range results {
		for _, match := range result.Matches {
			for _, text := range renderer.matcher.FindAllString(match.Text, -1) {
				if text == "" {
					continue
				}
				if err := renderer.output.writeString(fmt.Sprintf("%s:%d:%s\n", result.Path, match.Line, text)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func compileOnlyMatcher(options *Options) *regexp.Regexp {
	pattern := options.Params.Query
	if !options.Params.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if options.Params.IgnoreCase {
		pattern = "(?i)" + pattern
	}
	matcher, _ := regexp.Compile(pattern) // validated by the search engine
	return matcher
}
