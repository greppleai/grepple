package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
)

type segmentRenderer struct {
	output       *outputWriter
	anchors      anchorLookup
	contextGuard *segmentContextGuard
}

func (renderer segmentRenderer) Render(results []api.FileResult) error {
	if analysis := searchSourceAnalysis(results); analysis != nil && analysis.Unsupported+analysis.Failed+analysis.Recovered > 0 {
		message := fmt.Sprintf("! incomplete source analysis returned=%d structured=%d plain=%d unsupported=%d failed=%d recovered=%d\n\n", analysis.Returned, analysis.Structured, analysis.Plain, analysis.Unsupported, analysis.Failed, analysis.Recovered)
		if err := renderer.output.writeString(message); err != nil {
			return err
		}
	}
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
	return renderer.renderRelatedTypeDefinitions(collectRelatedTypeDefinitions(results))
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
		if err := renderer.renderSegment(resultSourceIdentity(result), result.Path, nil, segment, width, "", ""); err != nil {
			return err
		}
		cursor = segment.End + 1
	}
	line := relatedRootLine(result)
	return renderer.renderRelated(result.Related, result.OmittedRelatedCallers, result.OmittedRelatedCallees, result.OmittedRelatedTypes, result.Path, line)
}

func relatedRootLine(result api.FileResult) int {
	if len(result.Segments) > 0 && result.Segments[0].Start > 0 {
		return result.Segments[0].Start
	}
	if len(result.Matches) > 0 {
		if result.Matches[0].StartLine > 0 {
			return result.Matches[0].StartLine
		}
		if result.Matches[0].Line > 0 {
			return result.Matches[0].Line
		}
	}
	return 1
}

func (renderer segmentRenderer) renderRelated(related []api.RelatedSymbol, omittedCallers, omittedCallees, omittedTypes int, path string, line int) error {
	if len(related) == 0 && omittedCallers == 0 && omittedCallees == 0 && omittedTypes == 0 {
		return nil
	}
	if err := renderer.output.writeString("\nNext points (code navigation):\n"); err != nil {
		return err
	}
	if err := renderer.renderRelatedPoints(related, 1, true); err != nil {
		return err
	}
	return renderer.renderRelatedOmissions(omittedCallers, omittedCallees, omittedTypes, path, line, 1)
}

func (renderer segmentRenderer) renderRelatedPoints(related []api.RelatedSymbol, depth int, includeTypes bool) error {
	for _, point := range related {
		if point.Direction == "type" && !includeTypes {
			continue
		}
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
	locationPath, label, suffix := relatedPointPresentation(point)
	location := fmt.Sprintf("%s:%d-%d", locationPath, point.Start, point.End)
	if point.Confidence == "dependency-unresolved" && point.Artifact == nil {
		location = locationPath
	}
	line := fmt.Sprintf("%s%s %s  %s  %s%s\n", indent, arrow, point.Name, location, label, suffix)
	if err := renderer.output.writeString(line); err != nil {
		return err
	}
	if point.Direction == "type" {
		return nil
	}
	calls := relatedCallPoints(point.Related)
	if len(calls) > 0 || point.OmittedCallers > 0 || point.OmittedCallees > 0 {
		if err := renderer.output.writeString(strings.Repeat("  ", depth+1) + "next:\n"); err != nil {
			return err
		}
		if err := renderer.renderRelatedPoints(calls, depth+2, false); err != nil {
			return err
		}
		return renderer.renderRelatedOmissions(point.OmittedCallers, point.OmittedCallees, 0, point.Path, point.Start, depth+2)
	}
	return nil
}

func relatedCallPoints(points []api.RelatedSymbol) []api.RelatedSymbol {
	calls := make([]api.RelatedSymbol, 0, len(points))
	for _, point := range points {
		if point.Direction != "type" {
			calls = append(calls, point)
		}
	}
	return calls
}

func relatedPointPresentation(point api.RelatedSymbol) (string, string, string) {
	locationPath := point.Path
	suffix := ""
	if point.Artifact != nil {
		identity := navigationArtifactProvenance(point.Artifact)
		if point.Artifact.Repository != "" {
			locationPath = point.Artifact.Repository + ":" + point.Path
		}
		suffix = " [" + point.Confidence + "; " + identity + "]"
	} else if point.Confidence == "candidate" {
		suffix = fmt.Sprintf(" [candidate; try --at %s:%d]", point.Path, point.Start)
	} else if point.Confidence == "dependency-unresolved" && point.External != nil {
		suffix = dependencyUnresolvedSuffix(*point.External)
	}
	label := fmt.Sprintf("call:%d", point.CallLine)
	if point.Direction == "type" {
		role := point.Role
		if role == "" {
			role = "used"
		}
		label = fmt.Sprintf("%s-type:%d", role, point.CallLine)
	}
	return locationPath, label, suffix
}

func navigationArtifactProvenance(artifact *api.NavigationArtifactIdentity) string {
	parts := []string{artifact.Module + "@" + artifact.Version}
	if artifact.Source != "" {
		parts = append(parts, "source "+artifact.Source)
	}
	if artifact.Commit != "" {
		parts = append(parts, "commit "+artifact.Commit)
	}
	if artifact.Integrity != "" {
		parts = append(parts, "sum "+artifact.Integrity)
	}
	return strings.Join(parts, "; ")
}

func dependencyUnresolvedSuffix(reference api.ExternalNavigationReference) string {
	identity := reference.ImportPath
	if reference.Version != "" {
		identity = reference.Module + "@" + reference.Version
	} else if len(reference.Candidates) > 0 {
		candidates := make([]string, 0, len(reference.Candidates))
		for _, candidate := range reference.Candidates {
			candidates = append(candidates, candidate.Ecosystem+":"+candidate.Module+"@"+candidate.Version)
		}
		identity += "; candidates " + strings.Join(candidates, ", ")
	}
	suffix := " [dependency-unresolved; " + identity
	if reference.Source != "" {
		suffix += "; source " + reference.Source
	}
	if reference.Integrity != "" {
		suffix += "; sum " + reference.Integrity
	}
	return suffix + "]"
}

func (renderer segmentRenderer) renderRelatedOmissions(callers, callees, types int, path string, line, depth int) error {
	if callers == 0 && callees == 0 && types == 0 {
		return nil
	}
	indent := strings.Repeat("  ", depth)
	if callers > 0 || callees > 0 {
		parts := make([]string, 0, 2)
		if callees > 0 {
			parts = append(parts, fmt.Sprintf("%d additional %s", callees, pluralizeRelated("callee", callees)))
		}
		if callers > 0 {
			parts = append(parts, fmt.Sprintf("%d additional %s", callers, pluralizeRelated("caller", callers)))
		}
		direction := relatedGraphDirection(callers, callees)
		location := fmt.Sprintf("%s:%d", path, line)
		command := fmt.Sprintf("grepple graph %s --at %s --depth 2 --json .", direction, quoteCommandArgument(location))
		if err := renderer.output.writeString(fmt.Sprintf("%s… %s omitted; continue: %s …\n", indent, strings.Join(parts, " and "), command)); err != nil {
			return err
		}
	}
	if types > 0 {
		return renderer.output.writeString(fmt.Sprintf("%s… %d additional %s omitted\n", indent, types, pluralizeRelated("type declaration", types)))
	}
	return nil
}

func relatedGraphDirection(callers, callees int) string {
	if callers > 0 && callees == 0 {
		return "callers"
	}
	if callees > 0 && callers == 0 {
		return "callees"
	}
	return "impact"
}

func quoteCommandArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'\"") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func pluralizeRelated(noun string, count int) string {
	if count == 1 {
		return noun
	}
	return noun + "s"
}

func (renderer segmentRenderer) renderSegment(source, path string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment, width int, prefix, contextLabel string) error {
	if segment.Kind == "spacing" {
		return renderer.output.writeString(strings.Repeat("\n", segment.End-segment.Start+1))
	}
	if segment.Kind == "summary" {
		return renderer.output.writeString(fmt.Sprintf("%s%*d   %s\n", prefix, width, segment.Start, segment.Text))
	}
	if renderer.contextGuard.seen(source, artifact, segment) {
		marker := prefix + segmentContextMarker(source, segment, contextLabel)
		renderer.contextGuard.recordMarker(len(marker))
		return renderer.output.writeString(marker)
	}
	if err := renderer.renderSegmentLines(source, path, artifact, segment, width, prefix); err != nil {
		return err
	}
	renderer.contextGuard.record(source, artifact, segment)
	return nil
}

func (renderer segmentRenderer) renderSegmentLines(source, path string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment, width int, prefix string) error {
	texts := strings.Split(segment.Text, "\n")
	lines := make([]contextSourceLine, len(texts))
	for index, text := range texts {
		lines[index] = contextSourceLine{number: segment.Start + index, text: text}
	}
	if artifact != nil {
		return renderer.renderExpandedSegmentLines(path, lines, width, prefix)
	}
	for _, run := range contextLineRuns(renderer.contextGuard, source, lines) {
		if err := renderer.renderSegmentRun(path, lines, run, width, prefix); err != nil {
			return err
		}
	}
	return nil
}

func (renderer segmentRenderer) renderSegmentRun(path string, lines []contextSourceLine, run contextLineRun, width int, prefix string) error {
	if !run.omit {
		return renderer.renderExpandedSegmentLines(path, lines[run.start:run.end], width, prefix)
	}
	if err := renderer.output.writeString(renderer.segmentLineRow(path, lines[run.start].number, lines[run.start].text, width, prefix)); err != nil {
		return err
	}
	omitted := lines[run.start+1 : run.end-1]
	marker := fmt.Sprintf("%s… lines %d-%d omitted; unchanged anchored source already exists in context …\n", prefix, omitted[0].number, omitted[len(omitted)-1].number)
	if err := renderer.output.writeString(marker); err != nil {
		return err
	}
	sourceBytes, renderedBytes := renderer.segmentRunBytes(path, omitted, width, prefix)
	renderer.contextGuard.recordLineRangeOmission(len(omitted), sourceBytes, renderedBytes, len(marker))
	last := lines[run.end-1]
	return renderer.output.writeString(renderer.segmentLineRow(path, last.number, last.text, width, prefix))
}

func (renderer segmentRenderer) renderExpandedSegmentLines(path string, lines []contextSourceLine, width int, prefix string) error {
	for _, line := range lines {
		if err := renderer.output.writeString(renderer.segmentLineRow(path, line.number, line.text, width, prefix)); err != nil {
			return err
		}
	}
	return nil
}

func (renderer segmentRenderer) segmentRunBytes(path string, lines []contextSourceLine, width int, prefix string) (int, int) {
	sourceBytes, renderedBytes := 0, 0
	for _, line := range lines {
		sourceBytes += len(line.text)
		renderedBytes += len(renderer.segmentLineRow(path, line.number, line.text, width, prefix))
	}
	return sourceBytes, renderedBytes
}

func resultSourceIdentity(result api.FileResult) string {
	if result.Repo == "" {
		return result.Path
	}
	return result.Repo + "\x00" + result.Path
}

func (renderer segmentRenderer) renderSourceLine(path string, line int, content string, width int) error {
	return renderer.output.writeString(renderer.segmentLineRow(path, line, content, width, ""))
}

func (renderer segmentRenderer) segmentLineRow(path string, line int, content string, width int, prefix string) string {
	content = normalizeRenderedAnchorLine(content)
	if prefix != "" {
		return fmt.Sprintf("%s%*d   %s\n", prefix, width, line, content)
	}
	if anchor := renderer.anchors.line(path, line); anchor != "" {
		return fmt.Sprintf("%s%s%d%s%s\n", anchor, anchorOutputSeparator, line, anchorOutputSeparator, content)
	}
	return fmt.Sprintf("%*d   %s\n", width, line, content)
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
