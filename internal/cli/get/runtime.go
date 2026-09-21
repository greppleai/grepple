// Package get implements indexed source-file retrieval.
package get

import (
	"io"
	"net/http"
	"os"

	"github.com/greppleai/grepple/linerange"
	"github.com/greppleai/grepple/parser"
)

// RequestFactory constructs an optionally authenticated HTTP request.
type RequestFactory func(method, target, contentType string, body io.Reader) (*http.Request, error)

// Dependencies supplies process-owned command services.
type Dependencies struct {
	Stdout             io.Writer
	Stderr             io.Writer
	ServerDefault      func(string) string
	NewRequest         RequestFactory
	RequestExit        func(int)
	RecordRangeOutcome func(linerange.Outcome)
	ReportRangeError   func(error) error
	FullMissError      func(error) error
	RenderOutline      func(parser.FileOutline, string) string
}

func (dependencies Dependencies) stdout() io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
}
func (dependencies Dependencies) stderr() io.Writer {
	if dependencies.Stderr != nil {
		return dependencies.Stderr
	}
	return os.Stderr
}
func (dependencies Dependencies) serverDefault(value string) string {
	if dependencies.ServerDefault != nil {
		return dependencies.ServerDefault(value)
	}
	return value
}
func (dependencies Dependencies) newRequest(method, target, contentType string, body io.Reader) (*http.Request, error) {
	if dependencies.NewRequest != nil {
		return dependencies.NewRequest(method, target, contentType, body)
	}
	return http.NewRequest(method, target, body)
}
func (dependencies Dependencies) requestExit(code int) {
	if dependencies.RequestExit != nil {
		dependencies.RequestExit(code)
	}
}
func (dependencies Dependencies) recordRangeOutcome(outcome linerange.Outcome) {
	if dependencies.RecordRangeOutcome != nil {
		dependencies.RecordRangeOutcome(outcome)
	}
}
func (dependencies Dependencies) reportRangeError(err error) error {
	if dependencies.ReportRangeError != nil {
		return dependencies.ReportRangeError(err)
	}
	return err
}
func (dependencies Dependencies) fullMissError(err error) error {
	if dependencies.FullMissError != nil {
		return dependencies.FullMissError(err)
	}
	return err
}
func (dependencies Dependencies) renderOutline(outline parser.FileOutline, content string) string {
	if dependencies.RenderOutline != nil {
		return dependencies.RenderOutline(outline, content)
	}
	return content
}
