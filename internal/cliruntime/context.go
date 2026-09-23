// Package cliruntime defines the shared command execution protocol and runtime.
package cliruntime

import (
	"io"
	"os"

	"github.com/greppleai/grepple/internal/apiclient"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/usersettings"
)

// Configuration exposes immutable invocation configuration to commands.
type Configuration interface {
	ServerDefault(string) string
	ContextGuardEnabled() bool
	InlineOutputThreshold() int
	UserSettings() (usersettings.Config, error)
}

// Repository exposes the repository invocation context. Command-specific source
// loaders and projections are built by the commands from these primitives.
type Repository interface {
	WorkingDirectory() string
	Current() string
	ScopeOptions() (sourcedomain.Options, error)
	AppendScopeFlags([]string) []string
	InvocationOptions() RepositoryInvocationOptions
	CacheDirectory() string
	ConfigurationPath() (string, error)
}

// RepositoryInvocationOptions are the repository-wide flags active for one invocation.
type RepositoryInvocationOptions struct {
	NoRepositoryConfig bool
	NoConfigIgnore     bool
	ProductionOnly     bool
}

// Context is the common invocation environment supplied to every command.
type Context interface {
	Stdin() io.Reader
	Stdout() io.Writer
	Stderr() io.Writer
	APIClient() apiclient.APIClient
	Configuration() Configuration
	Repository() Repository
	RequestExit(int)
}

// Environment is the default Context implementation used by the CLI composition root.
type Environment struct {
	Input             io.Reader
	Output            io.Writer
	ErrorOutput       io.Writer
	Client            apiclient.APIClient
	Config            Configuration
	RepositoryContext Repository
	Exit              func(int)
}

func (context Environment) Stdin() io.Reader {
	if context.Input != nil {
		return context.Input
	}
	return os.Stdin
}

func (context Environment) Stdout() io.Writer {
	if context.Output != nil {
		return context.Output
	}
	return os.Stdout
}

func (context Environment) Stderr() io.Writer {
	if context.ErrorOutput != nil {
		return context.ErrorOutput
	}
	return os.Stderr
}

func (context Environment) APIClient() apiclient.APIClient {
	if context.Client != nil {
		return context.Client
	}
	return apiclient.New(apiclient.WithErrorOutput(context.Stderr()))
}

func (context Environment) Configuration() Configuration {
	if context.Config != nil {
		return context.Config
	}
	return ConfigurationServices{}
}
func (context Environment) Repository() Repository {
	if context.RepositoryContext != nil {
		return context.RepositoryContext
	}
	return RepositoryServices{}
}

func (context Environment) RequestExit(code int) {
	if context.Exit != nil {
		context.Exit(code)
	}
}
