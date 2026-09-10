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

var errOutputTruncated = errors.New("output truncated")

const outputTruncationMarker = "\n… grepple output truncated; narrow the path/glob or use --limit, --line-only, -l, or --count (--max-output-bytes 0 disables this cap) …\n"

type outputWriter struct {
	writer   io.Writer
	maxBytes int
	written  int
}

func newOutputWriter(writer io.Writer) *outputWriter {
	return &outputWriter{writer: writer}
}

func newBoundedOutputWriter(writer io.Writer, maxBytes int) *outputWriter {
	return &outputWriter{writer: writer, maxBytes: maxBytes}
}

func stdoutWriter() *outputWriter {
	return newOutputWriter(os.Stdout)
}

func (output *outputWriter) writeString(value string) error {
	if output.maxBytes <= 0 {
		return output.writeDirect(value)
	}
	marker := boundedOutputMarker(output.maxBytes)
	contentLimit := output.maxBytes - len(marker)
	if output.written+len(value) <= contentLimit {
		return output.writeDirect(value)
	}
	remaining := contentLimit - output.written
	if remaining > 0 {
		prefix := truncateOutputPrefix(value, remaining)
		if err := output.writeDirect(prefix); err != nil {
			return err
		}
	}
	if err := output.writeDirect(marker); err != nil {
		return err
	}
	return errOutputTruncated
}

func (output *outputWriter) writeDirect(value string) error {
	written, err := io.WriteString(output.writer, value)
	output.written += written
	if errors.Is(err, syscall.EPIPE) {
		return errBrokenPipe{}
	}
	return err
}

func boundedOutputMarker(maxBytes int) string {
	if len(outputTruncationMarker) <= maxBytes {
		return outputTruncationMarker
	}
	const shortMarker = "…\n"
	if len(shortMarker) <= maxBytes {
		return shortMarker
	}
	return ""
}

func truncateOutputPrefix(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	prefix := value[:limit]
	if newline := bytes.LastIndexByte([]byte(prefix), '\n'); newline >= 0 {
		return prefix[:newline+1]
	}
	return ""
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
