package cli

import "github.com/greppleai/grepple/api"

type resultRenderer interface {
	Render([]api.FileResult) error
}

func newResultRenderer(options *cliOptions, output *outputWriter) resultRenderer {
	switch {
	case options.Params.Files || options.FilesWithMatches:
		return filesRenderer{output: output, json: options.JSON != "off"}
	case options.Count:
		return countRenderer{output: output, json: options.JSON != "off"}
	case options.JSON != "off":
		return jsonResultRenderer{output: output, matchesOnly: options.JSON == "matches" || options.LineOnly}
	case options.Params.BeforeContext > 0 || options.Params.AfterContext > 0:
		return contextRenderer{output: output}
	case options.OnlyMatching:
		return onlyMatchingRenderer{output: output, maxLines: options.Params.MaxSegments, matcher: compileOnlyMatcher(options)}
	case options.LineOnly:
		return lineRenderer{output: output, maxLines: options.Params.MaxSegments}
	default:
		return segmentRenderer{output: output}
	}
}

func renderResults(options *cliOptions, results []api.FileResult) error {
	renderer := newResultRenderer(options, stdoutWriter())
	if err := renderer.Render(results); err != nil {
		return err
	}
	return setSearchExit(results)
}
