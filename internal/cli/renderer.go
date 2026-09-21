package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/greppleai/grepple/api"
	rendercommand "github.com/greppleai/grepple/internal/cli/render"
	"github.com/greppleai/grepple/linerange"
)

func rendererOptions(options *cliOptions) rendercommand.Options {
	return rendercommand.Options{Params: options.Params, LineOnly: options.LineOnly, OnlyMatching: options.OnlyMatching, JSON: options.JSON, Count: options.Count, FilesWithMatches: options.FilesWithMatches, MaxOutputBytes: options.MaxOutputBytes, RepeatSource: options.RepeatSource, Anchors: rendercommand.AnchorLookup(options.AnchorLines), Metadata: options.ResultMetadata}
}

func outputForOptions(options *cliOptions) *outputWriter {
	output := stdoutWriter()
	if options.JSON == "off" && options.MaxOutputBytes > 0 {
		return newBoundedOutputWriter(output.writer, options.MaxOutputBytes)
	}
	return output
}

type renderContextGuard struct{ guard *segmentContextGuard }

func (adapter renderContextGuard) Seen(source string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment) bool {
	return adapter.guard.seen(source, artifact, segment)
}
func (adapter renderContextGuard) Record(source string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment) {
	adapter.guard.record(source, artifact, segment)
}
func (adapter renderContextGuard) RecordMarker(bytes int) { adapter.guard.recordMarker(bytes) }
func (adapter renderContextGuard) RecordSearchLine(source string, line int, text string) {
	adapter.guard.recordSearchLine(source, line, text)
}
func (adapter renderContextGuard) RecordLineRangeOmission(lines, sourceBytes, renderedBytes, markerBytes int) {
	adapter.guard.recordLineRangeOmission(lines, sourceBytes, renderedBytes, markerBytes)
}
func (adapter renderContextGuard) LineCovered(source string, line int, text string) bool {
	return adapter.guard.lineCovered(source, line, text)
}

func renderResults(options *cliOptions, results []api.FileResult) error {
	warnLineRangeResults(results)
	guard := contextGuardForResults(options, results)
	var renderGuard rendercommand.ContextGuard
	if guard != nil {
		renderGuard = renderContextGuard{guard: guard}
	}
	written, err := rendercommand.Render(rendererOptions(options), results, os.Stdout, renderGuard)
	if guard != nil {
		guard.returnedBytes = written
	}
	guard.close()
	if err != nil {
		return err
	}
	return setSearchExit(results)
}

func warnLineRangeResults(results []api.FileResult) {
	seen := map[string]bool{}
	for _, result := range results {
		if result.LineRange == nil || result.LineRange.Warning == "" {
			continue
		}
		warning := result.Path + ": " + result.LineRange.Warning
		if !seen[warning] {
			fmt.Fprintln(os.Stderr, "warning:", warning)
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

func contextGuardForResults(options *cliOptions, results []api.FileResult) *segmentContextGuard {
	if activeInlineOutputThreshold < 1 || !contextGuardEnabled() {
		return nil
	}
	guard, err := openSegmentContextGuard()
	if err != nil {
		return nil
	}
	guard.observed = true
	guard.resultFiles = len(results)
	recordGuardLineRangeOutcomes(guard, results)
	segmentMode := segmentOutputMode(options)
	focusedLineMode := focusedLineCoverageMode(options)
	broadLineMode := broadLineCoverageProducerMode(options)
	enclosingMode := enclosingLineCoverageProducerMode(options)
	jsonMode := jsonCoverageProducerMode(options)
	contextMode := contextCoverageProducerMode(options)
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
	_, err = rendercommand.Render(rendererOptions(options), results, &rendered, nil)
	if err != nil {
		guard.deduplicate = false
		guard.recordSegments = false
		guard.recordLines = false
		guard.bypassReason = "render-error"
		return guard
	}
	headroom := completeResultSegmentCount(results, jsonMode)*128 + focusedLineResultCount(options, results)*128
	if rendered.Len()+headroom > activeInlineOutputThreshold {
		guard.deduplicate = false
		guard.recordSegments = false
		guard.recordLines = false
		guard.bypassReason = "potential-spill"
	}
	if options.RepeatSource {
		guard.bypassRequested = true
		if guard.bypassReason != "potential-spill" {
			guard.deduplicate = false
			guard.recordSegments = segmentMode
			guard.recordLines = focusedLineMode
			guard.bypassReason = "requested"
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

func focusedLineCoverageMode(options *cliOptions) bool {
	return options.LineOnly && options.Params.At != "" && options.AnchorLines != nil
}

func broadLineCoverageProducerMode(options *cliOptions) bool {
	return options.LineOnly && options.Params.At == "" && !options.Params.EnclosingRanges && options.AnchorLines != nil
}

func contextCoverageProducerMode(options *cliOptions) bool {
	return (options.Params.BeforeContext > 0 || options.Params.AfterContext > 0) && options.AnchorLines != nil
}
func jsonCoverageProducerMode(options *cliOptions) bool {
	return options.JSON == "full" && !options.Stdin
}

func enclosingLineCoverageProducerMode(options *cliOptions) bool {
	return options.JSON == "off" && options.LineOnly && options.Params.EnclosingRanges && !options.Stdin
}

func focusedLineResultCount(options *cliOptions, results []api.FileResult) int {
	if !focusedLineCoverageMode(options) {
		return 0
	}
	count := 0
	for _, result := range results {
		count += len(result.Matches)
	}
	return count
}

func segmentOutputMode(options *cliOptions) bool {
	return options.JSON == "off" && !options.Params.Files && !options.FilesWithMatches && !options.Count &&
		options.Params.BeforeContext == 0 && options.Params.AfterContext == 0 && !options.OnlyMatching && !options.LineOnly
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
