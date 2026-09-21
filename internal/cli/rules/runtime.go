package rules

import (
	"fmt"
	"io"
	"net/http"

	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type commonArgs = cliruntime.CommonArgs

// RequestFactory constructs an authenticated HTTP request.
type RequestFactory func(method, target, contentType string, body io.Reader) (*http.Request, error)

// Dependencies supplies remote transport and process exit state.
type Dependencies struct {
	ServerDefault func(string) string
	NewRequest    RequestFactory
	RequestExit   func(int)
}

type command struct{ dependencies Dependencies }

// New constructs the rules command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

func (d Dependencies) serverDefault(value string) string {
	if d.ServerDefault != nil {
		return d.ServerDefault(value)
	}
	return value
}
func (d Dependencies) newRequest(method, target, contentType string, body io.Reader) (*http.Request, error) {
	if d.NewRequest == nil {
		return nil, fmt.Errorf("rules request transport is unavailable")
	}
	return d.NewRequest(method, target, contentType, body)
}
func (d Dependencies) requestExit(code int) {
	if d.RequestExit != nil {
		d.RequestExit(code)
	}
}
func readGritQuery(reader io.Reader) (string, error) {
	content, err := io.ReadAll(io.LimitReader(reader, api.MaxGritQueryBytes+1))
	if err != nil {
		return "", err
	}
	if len(content) > api.MaxGritQueryBytes {
		return "", fmt.Errorf("structural query exceeds the %d-byte maximum", api.MaxGritQueryBytes)
	}
	return string(content), nil
}
