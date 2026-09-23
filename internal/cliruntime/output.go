package cliruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"syscall"
)

// ErrOutputTruncated reports that a bounded writer emitted its truncation marker.
var ErrOutputTruncated = errors.New("output truncated")

const outputTruncationMarker = "\n… grepple output truncated; narrow the path/glob or use --limit, --line-only, -l, or --count (--max-output-bytes 0 disables this cap) …\n"

// BrokenPipeError preserves EPIPE while giving CLI output a stable error type.
type BrokenPipeError struct{}

func (BrokenPipeError) Error() string { return "broken pipe" }
func (BrokenPipeError) Unwrap() error { return syscall.EPIPE }

// Output writes command output and optionally enforces a byte limit.
type Output struct {
	writer   io.Writer
	maxBytes int
	written  int
}

// NewOutput creates an unlimited command output writer.
func NewOutput(writer io.Writer) *Output {
	return &Output{writer: writer}
}

// NewBoundedOutput creates a command output writer capped at maxBytes.
func NewBoundedOutput(writer io.Writer, maxBytes int) *Output {
	return &Output{writer: writer, maxBytes: maxBytes}
}

// Written returns the number of bytes written, including a truncation marker.
func (output *Output) Written() int { return output.written }

// WriteString writes value, truncating at a complete line when bounded.
func (output *Output) WriteString(value string) error {
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
		if err := output.writeDirect(truncateOutputPrefix(value, remaining)); err != nil {
			return err
		}
	}
	if err := output.writeDirect(marker); err != nil {
		return err
	}
	return ErrOutputTruncated
}

func (output *Output) writeDirect(value string) error {
	written, err := io.WriteString(output.writer, value)
	output.written += written
	if errors.Is(err, syscall.EPIPE) {
		return BrokenPipeError{}
	}
	return err
}

// WriteJSON writes indented JSON without HTML escaping.
func (output *Output) WriteJSON(value any) error {
	var rendered bytes.Buffer
	encoder := json.NewEncoder(&rendered)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return output.WriteString(rendered.String())
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
