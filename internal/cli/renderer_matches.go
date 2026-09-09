package cli

import (
	"fmt"
	"regexp"

	"github.com/greppleai/grepple/api"
)

type lineRenderer struct {
	output   *outputWriter
	maxLines int
}

func (renderer lineRenderer) Render(results []api.FileResult) error {
	for _, result := range results {
		limit := min(len(result.Matches), renderer.maxLines)
		for _, match := range result.Matches[:limit] {
			if err := renderer.output.writeString(fmt.Sprintf("%s:%d:%s\n", result.Path, match.Line, match.Text)); err != nil {
				return err
			}
		}
	}
	return nil
}

type onlyMatchingRenderer struct {
	output   *outputWriter
	maxLines int
	matcher  *regexp.Regexp
}

func (renderer onlyMatchingRenderer) Render(results []api.FileResult) error {
	for _, result := range results {
		limit := min(len(result.Matches), renderer.maxLines)
		for _, match := range result.Matches[:limit] {
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
