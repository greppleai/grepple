// Package languages reports registered language capabilities.
package languages

import (
	"io"
	"os"
)

// Dependencies supplies process-owned command services.
type Dependencies struct{ Stdout io.Writer }

func (dependencies Dependencies) stdout() io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
}
