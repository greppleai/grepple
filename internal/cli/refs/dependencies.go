// Package refs implements indexed repository reference listing.
package refs

import (
	"io"
	"os"

	reposcommand "github.com/greppleai/grepple/internal/cli/repos"
)

// Dependencies supplies process-owned command services.
type Dependencies struct {
	Stdout        io.Writer
	ServerDefault func(string) string
	NewRequest    reposcommand.RequestFactory
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
func (dependencies Dependencies) requestExit(code int) {
	if dependencies.RequestExit != nil {
		dependencies.RequestExit(code)
	}
}
