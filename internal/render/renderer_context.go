package render

import (
	"fmt"
	"github.com/greppleai/grepple/search"
)

type contextRenderer struct {
	output       *outputWriter
	anchors      AnchorLookup
	contextGuard ContextGuard
}

func (renderer contextRenderer) Render(results []search.FileResult) error {
	if renderer.contextGuard == nil {
		renderer.contextGuard = noopContextGuard{}
	}
	contextPrinted := false
	for _, result := range results {
		didPrint, err := printContext(renderer.output, renderer.anchors, renderer.contextGuard, resultSourceIdentity(result), result.Path, result.Context, contextPrinted)
		if err != nil {
			return err
		}
		contextPrinted = contextPrinted || didPrint
	}
	return nil
}

func printContext(output *outputWriter, anchors AnchorLookup, guard ContextGuard, source, path string, lines []search.ContextLine, leading bool) (bool, error) {
	if anchors != nil {
		return printAnchoredContext(output, anchors, guard, source, path, lines, leading)
	}
	last := 0
	for index, line := range lines {
		if (index == 0 && leading) || (last > 0 && line.Line != last+1) {
			if err := output.writeString("--\n"); err != nil {
				return false, err
			}
		}
		separator := "-"
		if line.Match {
			separator = ":"
		}
		if err := output.writeString(fmt.Sprintf("%s%s%d%s%s\n", path, separator, line.Line, separator, line.Text)); err != nil {
			return false, err
		}
		last = line.Line
	}
	return len(lines) > 0, nil
}

func printAnchoredContext(output *outputWriter, anchors AnchorLookup, guard ContextGuard, source, path string, lines []search.ContextLine, leading bool) (bool, error) {
	if len(lines) == 0 {
		return false, nil
	}
	if leading {
		if err := output.writeString("\n"); err != nil {
			return false, err
		}
	}
	if err := output.writeString(path + "\n\n"); err != nil {
		return false, err
	}
	last := 0
	for _, line := range lines {
		if last > 0 && line.Line != last+1 {
			if err := output.writeString("--\n"); err != nil {
				return false, err
			}
		}
		row := fmt.Sprintf("%s%s%d%s%s\n", anchors.Line(path, line.Line), anchorOutputSeparator, line.Line, anchorOutputSeparator, normalizeRenderedAnchorLine(line.Text))
		if err := output.writeString(row); err != nil {
			return false, err
		}
		guard.RecordSearchLine(source, line.Line, line.Text)
		last = line.Line
	}
	return true, nil
}
