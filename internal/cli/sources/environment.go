// Package sources explains repository source-selection decisions.
package sources

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/cliruntime"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

// Environment contains repository configuration needed for source selection.
type Environment = sourcedomain.Environment

// Dependencies supplies process-owned repository and output services.
type Dependencies struct {
	Stdout           io.Writer
	Environment      func() (Environment, error)
	WorkingDirectory func() string
}

func dependenciesFromApplication(application cliruntime.Context) Dependencies {
	return Dependencies{
		Stdout:           application.Stdout(),
		WorkingDirectory: application.Repository().WorkingDirectory,
		Environment: func() (Environment, error) {
			scope, err := application.Repository().ScopeOptions()
			if err != nil {
				return Environment{}, err
			}
			configPath, err := application.Repository().ConfigurationPath()
			if err != nil {
				return Environment{}, err
			}
			root := scope.IgnoreRoot
			if root == "" {
				root = scope.WorkingDirectory
			}
			if configPath != "" {
				root = filepath.Dir(configPath)
			}
			invocation := application.Repository().InvocationOptions()
			return Environment{Root: root, ConfigPath: configPath, IgnorePaths: append([]string(nil), scope.IgnorePaths...), IgnoreDisabled: invocation.NoConfigIgnore, ProductionOnly: invocation.ProductionOnly}, nil
		},
	}
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
