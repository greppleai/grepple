package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
)

type segmentRenderer struct {
	output *outputWriter
}

func (renderer segmentRenderer) Render(results []api.FileResult) error {
	for index, result := range results {
		if index > 0 {
			if err := renderer.output.writeString("\n"); err != nil {
				return err
			}
		}
		if err := renderer.renderFile(result); err != nil {
			return err
		}
	}
	return nil
}

func (renderer segmentRenderer) renderFile(result api.FileResult) error {
	if err := renderer.output.writeString(result.Path + "\n\n"); err != nil {
		return err
	}
	width := segmentLineWidth(result.Segments)
	sort.Slice(result.Segments, func(i, j int) bool {
		return result.Segments[i].Start < result.Segments[j].Start
	})
	cursor := 1
	for _, segment := range result.Segments {
		if segment.Start > cursor {
			if err := renderer.output.writeString(collapsedLines(segment.Start - cursor)); err != nil {
				return err
			}
		}
		if err := renderer.renderSegment(segment, width); err != nil {
			return err
		}
		cursor = segment.End + 1
	}
	return nil
}

func (renderer segmentRenderer) renderSegment(segment api.ResultSegment, width int) error {
	if segment.Kind == "summary" {
		return renderer.output.writeString(fmt.Sprintf("%*d   %s\n", width, segment.Start, segment.Text))
	}
	for index, line := range strings.Split(segment.Text, "\n") {
		if err := renderer.output.writeString(fmt.Sprintf("%*d   %s\n", width, segment.Start+index, line)); err != nil {
			return err
		}
	}
	return nil
}

func segmentLineWidth(segments []api.ResultSegment) int {
	width := 1
	for _, segment := range segments {
		if digits := len(fmt.Sprint(segment.End)); digits > width {
			width = digits
		}
	}
	return width
}

func collapsedLines(count int) string {
	if count <= 0 {
		return ""
	}
	word := "lines"
	if count == 1 {
		word = "line"
	}
	return fmt.Sprintf("\n// … %d %s collapsed …\n\n", count, word)
}
