package cli

import (
	"io"
	"os"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type errBrokenPipe = cliruntime.BrokenPipeError

var errOutputTruncated = cliruntime.ErrOutputTruncated

type outputWriter struct {
	writer   io.Writer
	delegate *cliruntime.Output
	written  int
}

func newOutputWriter(writer io.Writer) *outputWriter {
	return &outputWriter{writer: writer, delegate: cliruntime.NewOutput(writer)}
}

func newBoundedOutputWriter(writer io.Writer, maxBytes int) *outputWriter {
	return &outputWriter{writer: writer, delegate: cliruntime.NewBoundedOutput(writer, maxBytes)}
}

func stdoutWriter() *outputWriter {
	return newOutputWriter(os.Stdout)
}

func (output *outputWriter) writeString(value string) error {
	err := output.delegate.WriteString(value)
	output.written = output.delegate.Written()
	return err
}

func (output *outputWriter) writeJSON(value any) error {
	err := output.delegate.WriteJSON(value)
	output.written = output.delegate.Written()
	return err
}
