package render

import (
	"bytes"
	"fmt"
	"io"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/linerange"
)

// SearchOptions supplies result-formatting and output-context settings.
type SearchOptions struct {
	Options
	Output          io.Writer
	ErrorOutput     io.Writer
	ContextEnabled  bool
	InlineThreshold int
}

// Search renders search results and updates output-context coverage.
func Search(options SearchOptions, results []api.FileResult) error {
	warnLineRangeResults(results, options.ErrorOutput)
	guard := newContextGuard(options, results)
	written, err := Render(options.Options, results, options.Output, guard)
	if guard != nil {
		guard.returnedBytes = written
		guard.close()
	}
	return err
}

func warnLineRangeResults(results []api.FileResult, destination io.Writer) {
	if destination == nil {
		return
	}
	seen := map[string]bool{}
	for _, result := range results {
		if result.LineRange == nil || result.LineRange.Warning == "" {
			continue
		}
		warning := result.Path + ": " + result.LineRange.Warning
		if !seen[warning] {
			fmt.Fprintln(destination, "warning:", warning)
			seen[warning] = true
		}
	}
}

func recordGuardLineRangeOutcomes(guard *segmentContextGuard, results []api.FileResult) {
	for _, result := range results {
		if result.LineRange == nil {
			continue
		}
		switch linerange.Outcome(result.LineRange.Outcome) {
		case linerange.OutcomePartialMiss:
			guard.partialLineRangeMisses++
		case linerange.OutcomeFullMiss:
			guard.fullLineRangeMisses++
		}
	}
}

func newContextGuard(options SearchOptions, results []api.FileResult) *segmentContextGuard {
	if options.InlineThreshold < 1 || !options.ContextEnabled {
		return nil
	}
	guard, err := openSegmentContextGuard()
	if err != nil {
		return nil
	}
	guard.observed = true
	guard.resultFiles = len(results)
	recordGuardLineRangeOutcomes(guard, results)
	segmentMode := segmentOutputMode(options.Options)
	focusedLineMode := focusedLineCoverageMode(options.Options)
	broadLineMode := broadLineCoverageProducerMode(options.Options)
	enclosingMode := enclosingLineCoverageProducerMode(options.Options)
	jsonMode := jsonCoverageProducerMode(options.Options)
	contextMode := contextCoverageProducerMode(options.Options)
	guard.structuredRead = segmentMode
	guard.deduplicate = segmentMode || focusedLineMode
	guard.recordLines = focusedLineMode || broadLineMode || contextMode || enclosingMode || jsonMode
	guard.recordSegments = segmentMode || jsonMode
	guard.lineCoverageCall = focusedLineMode
	guard.bypassReason = initialReadBypassReason(segmentMode, focusedLineMode)
	if !contextCoverageEligible(segmentMode, focusedLineMode, broadLineMode, contextMode, enclosingMode, jsonMode) {
		return guard
	}
	var rendered bytes.Buffer
	_, err = Render(options.Options, results, &rendered, nil)
	if err != nil {
		guard.deduplicate, guard.recordSegments, guard.recordLines, guard.bypassReason = false, false, false, "render-error"
		return guard
	}
	headroom := completeResultSegmentCount(results, jsonMode)*128 + focusedLineResultCount(options.Options, results)*128
	if rendered.Len()+headroom > options.InlineThreshold {
		guard.deduplicate, guard.recordSegments, guard.recordLines, guard.bypassReason = false, false, false, "potential-spill"
	}
	if options.RepeatSource {
		guard.bypassRequested = true
		if guard.bypassReason != "potential-spill" {
			guard.deduplicate, guard.recordSegments, guard.recordLines, guard.bypassReason = false, segmentMode, focusedLineMode, "requested"
		}
	}
	return guard
}

func contextCoverageEligible(modes ...bool) bool {
	for _, enabled := range modes {
		if enabled {
			return true
		}
	}
	return false
}
func initialReadBypassReason(segmentMode, focusedLineMode bool) string {
	if segmentMode || focusedLineMode {
		return ""
	}
	return "non-structural"
}
func focusedLineCoverageMode(options Options) bool {
	return options.LineOnly && options.Params.At != "" && options.Anchors != nil
}
func broadLineCoverageProducerMode(options Options) bool {
	return options.LineOnly && options.Params.At == "" && !options.Params.EnclosingRanges && options.Anchors != nil
}
func contextCoverageProducerMode(options Options) bool {
	return (options.Params.BeforeContext > 0 || options.Params.AfterContext > 0) && options.Anchors != nil
}
func jsonCoverageProducerMode(options Options) bool { return options.JSON == "full" && !options.Stdin }
func enclosingLineCoverageProducerMode(options Options) bool {
	return options.JSON == "off" && options.LineOnly && options.Params.EnclosingRanges && !options.Stdin
}
func focusedLineResultCount(options Options, results []api.FileResult) int {
	if !focusedLineCoverageMode(options) {
		return 0
	}
	count := 0
	for _, result := range results {
		count += len(result.Matches)
	}
	return count
}
func segmentOutputMode(options Options) bool {
	return options.JSON == "off" && !options.Params.Files && !options.FilesWithMatches && !options.Count && options.Params.BeforeContext == 0 && options.Params.AfterContext == 0 && !options.OnlyMatching && !options.LineOnly
}
func completeResultSegmentCount(results []api.FileResult, includeAllRelated bool) int {
	count := 0
	for _, result := range results {
		count += completeSegmentCount(result.Segments)
		count += completeRelatedSegmentCount(result.Related, includeAllRelated)
	}
	return count
}
func completeRelatedSegmentCount(points []api.RelatedSymbol, includeAll bool) int {
	count := 0
	for _, point := range points {
		if includeAll || point.Direction == "type" {
			count += completeSegmentCount(point.Segments)
		}
		if includeAll {
			count += completeRelatedSegmentCount(point.Related, true)
		}
	}
	return count
}
func completeSegmentCount(segments []api.ResultSegment) int {
	count := 0
	for _, segment := range segments {
		if completeStructuralSegment(segment) {
			count++
		}
	}
	return count
}
