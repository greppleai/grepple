package cliruntime

import (
	"io"
	"os"

	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/config"
)

// ContextOptions configures the concrete command context.
type ContextOptions struct {
	Input                 io.Reader
	Output                io.Writer
	ErrorOutput           io.Writer
	Client                apiclient.APIClient
	Repository            RepositoryInvocationOptions
	Config                *config.Config
	InlineOutputThreshold int
	Exit                  func(int)
}

// NewContext creates the production context for one CLI invocation.
func NewContext(options ContextOptions) Context {
	input, output, diagnostics := options.Input, options.Output, options.ErrorOutput
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	if diagnostics == nil {
		diagnostics = os.Stderr
	}
	configuration := newConfigurationWithConfig(options.Repository, options.InlineOutputThreshold, options.Config)
	return Environment{
		Input:             input,
		Output:            output,
		ErrorOutput:       diagnostics,
		Client:            options.Client,
		Config:            configuration,
		RepositoryContext: newRepositoryWithConfig(options.Repository, diagnostics, options.Config),
		Exit:              options.Exit,
	}
}
