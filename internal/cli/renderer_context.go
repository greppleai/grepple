package cli

import (
	"fmt"

	"github.com/greppleai/grepple/api"
)

type contextRenderer struct {
	output *outputWriter
}

func (renderer contextRenderer) Render(results []api.FileResult) error {
	contextPrinted := false
	for _, result := range results {
		didPrint, err := printContext(renderer.output, result.Path, result.Context, contextPrinted)
		if err != nil {
			return err
		}
		contextPrinted = contextPrinted || didPrint
	}
	return nil
}

func printContext(output *outputWriter, path string, lines []api.ContextLine, leading bool) (bool, error) {
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
