package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/linerange"
)

type resultRenderer interface {
	Render([]api.FileResult) error
}

func newResultRenderer(options *cliOptions, output *outputWriter, guard *segmentContextGuard) resultRenderer {
	switch {
	case options.Params.Files || options.FilesWithMatches:
		return filesRenderer{output: output, json: options.JSON != "off"}
	case options.Count:
		return countRenderer{output: output, json: options.JSON != "off"}
	case options.JSON != "off":
		return jsonResultRenderer{output: output, matchesOnly: options.JSON == "matches" || options.LineOnly, metadata: options.ResultMetadata, contextGuard: guard}
	case options.Params.BeforeContext > 0 || options.Params.AfterContext > 0:
		return contextRenderer{output: output, anchors: options.AnchorLines, contextGuard: guard}
	case options.OnlyMatching:
		return onlyMatchingRenderer{output: output, matcher: compileOnlyMatcher(options)}
	case options.LineOnly:
		return lineRenderer{output: output, anchors: options.AnchorLines, contextGuard: guard, repeatSource: options.RepeatSource}
	default:
		return segmentRenderer{output: output, anchors: options.AnchorLines, contextGuard: guard}
	}
}

func outputForOptions(options *cliOptions) *outputWriter {
	output := stdoutWriter()
	if options.JSON == "off" && options.MaxOutputBytes > 0 {
		return newBoundedOutputWriter(output.writer, options.MaxOutputBytes)
	}
	return output
}

func renderResults(options *cliOptions, results []api.FileResult) error {
	warnLineRangeResults(results)
	guard := contextGuardForResults(options, results)
	output := outputForOptions(options)
	defer func() {
		if guard != nil {
			guard.returnedBytes = output.written
		}
		guard.close()
	}()
	renderer := newResultRenderer(options, output, guard)
	if err := renderer.Render(results); err != nil && !errors.Is(err, errOutputTruncated) {
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
	output := newOutputWriter(&rendered)
	if options.MaxOutputBytes > 0 {
		output = newBoundedOutputWriter(&rendered, options.MaxOutputBytes)
	}
	err = newResultRenderer(options, output, nil).Render(results)
	if err != nil && !errors.Is(err, errOutputTruncated) {
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
