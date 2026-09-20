// Package sources explains repository source-selection decisions.
package sources

import (
	"fmt"
	"io"
	"os"
)

// Environment contains repository configuration needed for source selection.
type Environment struct {
	Root           string
	ConfigPath     string
	IgnorePaths    []string
	IgnoreDisabled bool
	ProductionOnly bool
}

// Dependencies supplies process-owned repository and output services.
type Dependencies struct {
	Stdout           io.Writer
	Environment      func() (Environment, error)
	WorkingDirectory func() string
}

func (dependencies Dependencies) stdout() io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
}
func (dependencies Dependencies) environment() (Environment, error) {
	if dependencies.Environment == nil {
		return Environment{}, fmt.Errorf("source environment is unavailable")
	}
	return dependencies.Environment()
}
func (dependencies Dependencies) workingDirectory() string {
	if dependencies.WorkingDirectory != nil {
		return dependencies.WorkingDirectory()
	}
	value, _ := os.Getwd()
	return value
}
