// Package tree implements local and indexed-repository tree rendering.
package tree

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/greppleai/grepple/api"
)

// LocalTree builds a local source-tree response.
type LocalTree func(path string, depth int) (api.TreeResponse, error)

// RequestFactory constructs an optionally authenticated HTTP request.
type RequestFactory func(method, target, contentType string, body io.Reader) (*http.Request, error)

// Dependencies supplies process-owned command services.
type Dependencies struct {
	Stdout        io.Writer
	ServerDefault func(string) string
	NewRequest    RequestFactory
	RequestExit   func(int)
	LocalTree     LocalTree
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
func (dependencies Dependencies) localTree(path string, depth int) (api.TreeResponse, error) {
	if dependencies.LocalTree == nil {
		return api.TreeResponse{}, fmt.Errorf("local tree inspection is unavailable")
	}
	return dependencies.LocalTree(path, depth)
}

func (dependencies Dependencies) requestExit(code int) {
	if dependencies.RequestExit != nil {
		dependencies.RequestExit(code)
	}
}
