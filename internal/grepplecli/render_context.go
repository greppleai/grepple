package grepplecli

import (
	"fmt"
	"grepple/internal/api"
)

// PrintContext renders context lines in grep style (`path:line:text` for
// matches, `path-line-text` for context, `--` between gaps and before the
// first group when leading is set). It reports whether anything was printed.
func PrintContext(path string, lines []api.ContextLine, leading bool) (bool, error) {
	last := 0
	for index, line := range lines {
		if (index == 0 && leading) || (last > 0 && line.Line != last+1) {
			if err := SafeWrite("--\n"); err != nil {
				return false, err
			}
		}
		separator := "-"
		if line.Match {
			separator = ":"
		}
		if err := SafeWrite(fmt.Sprintf("%s%s%d%s%s\n", path, separator, line.Line, separator, line.Text)); err != nil {
			return false, err
		}
		last = line.Line
	}
	return len(lines) > 0, nil
}
