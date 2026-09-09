package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"
)

type errBrokenPipe struct{}

func (errBrokenPipe) Error() string { return "broken pipe" }

// Unwrap lets the process boundary recognize an early-closed pipe without
// depending on output implementation details.
func (errBrokenPipe) Unwrap() error { return syscall.EPIPE }

type outputWriter struct {
	writer io.Writer
}

func newOutputWriter(writer io.Writer) *outputWriter {
	return &outputWriter{writer: writer}
}

func stdoutWriter() *outputWriter {
	return newOutputWriter(os.Stdout)
}

func (output *outputWriter) writeString(value string) error {
	_, err := io.WriteString(output.writer, value)
	if errors.Is(err, syscall.EPIPE) {
		return errBrokenPipe{}
	}
	return err
}

func (output *outputWriter) writeJSON(value any) error {
	var rendered bytes.Buffer
	encoder := json.NewEncoder(&rendered)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return output.writeString(rendered.String())
}
