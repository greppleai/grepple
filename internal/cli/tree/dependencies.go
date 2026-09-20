// Package tree implements indexed-repository tree rendering.
package tree

import (
	"io"
	"net/http"
	"os"
)

// RequestFactory constructs an optionally authenticated HTTP request.
type RequestFactory func(method, target, contentType string, body io.Reader) (*http.Request, error)

// Dependencies supplies process-owned command services.
type Dependencies struct {
	Stdout        io.Writer
	ServerDefault func(string) string
	NewRequest    RequestFactory
	RequestExit   func(int)
}

func (dependencies Dependencies) stdout() io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
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
