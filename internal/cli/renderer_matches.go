package cli

import (
	"fmt"
	"regexp"

	"github.com/greppleai/grepple/api"
)

type lineRenderer struct {
	output  *outputWriter
	anchors anchorLookup
}

func (renderer lineRenderer) Render(results []api.FileResult) error {
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

func (renderer lineRenderer) renderAnchored(results []api.FileResult) error {
	for resultIndex, result := range results {
		if resultIndex > 0 {
			if err := renderer.output.writeString("\n"); err != nil {
				return err
			}
		}
		if err := renderer.output.writeString(result.Path + "\n\n"); err != nil {
			return err
		}
		for _, match := range result.Matches {
			anchor := renderer.anchors.line(result.Path, match.Line)
			row := fmt.Sprintf("%s%s%d%s%s\n", anchor, anchorOutputSeparator, match.Line, anchorOutputSeparator, normalizeRenderedAnchorLine(match.Text))
			if err := renderer.output.writeString(row); err != nil {
				return err
			}
		}
	}
	return nil
}

type onlyMatchingRenderer struct {
	output  *outputWriter
	matcher *regexp.Regexp
}

func (renderer onlyMatchingRenderer) Render(results []api.FileResult) error {
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

func compileOnlyMatcher(options *cliOptions) *regexp.Regexp {
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
