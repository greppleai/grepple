package examples

import (
	"io"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func runExamples(args []string, output io.Writer) error {
	return New(cliruntime.Environment{Output: output}).Run(args)
}
