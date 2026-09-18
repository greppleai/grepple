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
		return lineRenderer{output: output, anchors: options.AnchorLines}
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
	defer guard.close()
	renderer := newResultRenderer(options, outputForOptions(options), guard)
	if err := renderer.Render(results); err != nil && !errors.Is(err, errOutputTruncated) {
		return err
	}
	return setSearchExit(results)
}

func contextGuardForResults(options *cliOptions, results []api.FileResult) *segmentContextGuard {
	if !segmentOutputMode(options) || activeInlineOutputThreshold < 1 {
		return nil
	}
	var rendered bytes.Buffer
	output := newOutputWriter(&rendered)
	if options.MaxOutputBytes > 0 {
		output = newBoundedOutputWriter(&rendered, options.MaxOutputBytes)
	}
	err := newResultRenderer(options, output, nil).Render(results)
	if err != nil && !errors.Is(err, errOutputTruncated) {
		return nil
	}
	headroom := completeResultSegmentCount(results) * 128
	if rendered.Len()+headroom > activeInlineOutputThreshold {
		return nil
	}
	guard, err := openSegmentContextGuard()
	if err != nil {
		return nil
	}
	return guard
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
