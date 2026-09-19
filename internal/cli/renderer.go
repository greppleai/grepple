package cli

import (
	"bytes"
	"errors"

	"github.com/greppleai/grepple/api"
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
		return jsonResultRenderer{output: output, matchesOnly: options.JSON == "matches" || options.LineOnly, metadata: options.ResultMetadata}
	case options.Params.BeforeContext > 0 || options.Params.AfterContext > 0:
		return contextRenderer{output: output, anchors: options.AnchorLines}
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
	segmentMode := segmentOutputMode(options)
	focusedLineMode := focusedLineCoverageMode(options)
	guard.structuredRead = segmentMode
	if !segmentMode && !focusedLineMode {
		guard.deduplicate = false
		guard.recordSegments = false
		guard.bypassReason = "non-structural"
		return guard
	}
	guard.recordSegments = segmentMode
	guard.recordLines = focusedLineMode
	guard.lineCoverageCall = focusedLineMode
	var rendered bytes.Buffer
	output := newOutputWriter(&rendered)
	if options.MaxOutputBytes > 0 {
		output = newBoundedOutputWriter(&rendered, options.MaxOutputBytes)
	}
	err = newResultRenderer(options, output, nil).Render(results)
	if err != nil && !errors.Is(err, errOutputTruncated) {
		guard.deduplicate = false
		guard.recordSegments = false
		guard.bypassReason = "render-error"
		return guard
	}
	headroom := completeResultSegmentCount(results)*128 + focusedLineResultCount(options, results)*128
	if rendered.Len()+headroom > activeInlineOutputThreshold {
		guard.deduplicate = false
		guard.recordSegments = false
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

func focusedLineCoverageMode(options *cliOptions) bool {
	return options.LineOnly && options.Params.At != "" && options.AnchorLines != nil
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

func completeResultSegmentCount(results []api.FileResult) int {
	count := 0
	var countRelated func([]api.RelatedSymbol)
	countRelated = func(points []api.RelatedSymbol) {
		for _, point := range points {
			for _, segment := range point.Segments {
				if completeStructuralSegment(segment) {
					count++
				}
			}
			countRelated(point.Related)
		}
	}
	for _, result := range results {
		for _, segment := range result.Segments {
			if completeStructuralSegment(segment) {
				count++
			}
		}
		countRelated(result.Related)
	}
	return count
}
