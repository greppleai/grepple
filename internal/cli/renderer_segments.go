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
	return renderer.renderRelated(result.Related)
}
func (renderer segmentRenderer) renderRelated(related []api.RelatedSymbol) error {
	if len(related) == 0 {
		return nil
	}
	if err := renderer.output.writeString("\nNext points (code navigation):\n"); err != nil {
		return err
	}
	return renderer.renderRelatedPoints(related, 1)
}

func (renderer segmentRenderer) renderRelatedPoints(related []api.RelatedSymbol, depth int) error {
	for _, point := range related {
		if err := renderer.renderRelatedPoint(point, depth); err != nil {
			return err
		}
	}
	return nil
}

func (renderer segmentRenderer) renderRelatedPoint(point api.RelatedSymbol, depth int) error {
	indent := strings.Repeat("  ", depth)
	arrow := "→"
	if point.Direction == "caller" {
		arrow = "←"
	}
	suffix := ""
	if point.Confidence != "unique" {
		suffix = " [candidate]"
	}
	line := fmt.Sprintf("%s%s %s  %s:%d-%d  call:%d%s\n", indent, arrow, point.Name, point.Path, point.Start, point.End, point.CallLine, suffix)
	if err := renderer.output.writeString(line); err != nil {
		return err
	}
	if len(point.Segments) == 0 {
		return nil
	}
	if err := renderer.renderRelatedSegments(point.Segments, depth+1); err != nil {
		return err
	}
	if len(point.Related) > 0 {
		if err := renderer.output.writeString(strings.Repeat("  ", depth+1) + "next:\n"); err != nil {
			return err
		}
		return renderer.renderRelatedPoints(point.Related, depth+2)
	}
	return nil
}

func (renderer segmentRenderer) renderRelatedSegments(segments []api.ResultSegment, depth int) error {
	indent := strings.Repeat("  ", depth)
	width := segmentLineWidth(segments)
	for _, segment := range segments {
		for index, line := range strings.Split(segment.Text, "\n") {
			output := fmt.Sprintf("%s%*d   %s\n", indent, width, segment.Start+index, line)
			if err := renderer.output.writeString(output); err != nil {
				return err
			}
		}
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
