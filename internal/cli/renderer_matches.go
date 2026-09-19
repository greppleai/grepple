package cli

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/greppleai/grepple/api"
)

type lineRenderer struct {
	output       *outputWriter
	anchors      anchorLookup
	contextGuard *segmentContextGuard
	repeatSource bool
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
			renderer.contextGuard.recordSearchLine(resultSourceIdentity(result), match.Line, match.Text)
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
		if err := renderer.renderAnchoredResult(result); err != nil {
			return err
		}
	}
	return nil
}

func (renderer lineRenderer) renderAnchoredResult(result api.FileResult) error {
	matches := append([]api.ResultMatch(nil), result.Matches...)
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Line < matches[j].Line })
	lines := make([]contextSourceLine, len(matches))
	for index, match := range matches {
		lines[index] = contextSourceLine{number: match.Line, text: match.Text}
	}
	for _, run := range contextLineRuns(renderer.contextGuard, resultSourceIdentity(result), lines) {
		if err := renderer.renderAnchoredRun(result, matches, run); err != nil {
			return err
		}
	}
	return nil
}

func (renderer lineRenderer) renderAnchoredRun(result api.FileResult, matches []api.ResultMatch, run contextLineRun) error {
	if !run.omit || renderer.repeatSource {
		for _, match := range matches[run.start:run.end] {
			if err := renderer.renderAnchoredMatch(result, match); err != nil {
				return err
			}
		}
		return nil
	}
	if err := renderer.renderAnchoredMatch(result, matches[run.start]); err != nil {
		return err
	}
	omitted := matches[run.start+1 : run.end-1]
	marker := fmt.Sprintf("… lines %d-%d omitted; unchanged anchored source already exists in context …\n", omitted[0].Line, omitted[len(omitted)-1].Line)
	if err := renderer.output.writeString(marker); err != nil {
		return err
	}
	sourceBytes, renderedBytes := renderer.anchoredRunBytes(result, omitted)
	renderer.contextGuard.recordLineRangeOmission(len(omitted), sourceBytes, renderedBytes, len(marker))
	return renderer.renderAnchoredMatch(result, matches[run.end-1])
}

func (renderer lineRenderer) anchoredRunBytes(result api.FileResult, matches []api.ResultMatch) (int, int) {
	sourceBytes, renderedBytes := 0, 0
	for _, match := range matches {
		sourceBytes += len(match.Text)
		renderedBytes += len(renderer.anchoredMatchRow(result, match))
	}
	return sourceBytes, renderedBytes
}

func (renderer lineRenderer) renderAnchoredMatch(result api.FileResult, match api.ResultMatch) error {
	row := renderer.anchoredMatchRow(result, match)
	if err := renderer.output.writeString(row); err != nil {
		return err
	}
	renderer.contextGuard.recordSearchLine(resultSourceIdentity(result), match.Line, match.Text)
	return nil
}

func (renderer lineRenderer) anchoredMatchRow(result api.FileResult, match api.ResultMatch) string {
	anchor := renderer.anchors.line(result.Path, match.Line)
	return fmt.Sprintf("%s%s%d%s%s\n", anchor, anchorOutputSeparator, match.Line, anchorOutputSeparator, normalizeRenderedAnchorLine(match.Text))
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
