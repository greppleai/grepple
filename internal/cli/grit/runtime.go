// Package grit implements structural query commands.
package grit

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/gritql"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/search"
)

// DefaultTextOutputBytes is the default human-readable GritQL output cap.
const DefaultTextOutputBytes = 16 * 1024

type commonArgs = cliruntime.CommonArgs

var errOutputTruncated = cliruntime.ErrOutputTruncated

// Arguments exposes metadata-relevant command values.
type Arguments = gritArgs

// MetadataBuilder constructs normalized result metadata.
type MetadataBuilder func(values Arguments, response api.GritResponse, remote bool) *api.ResultMetadata

// Dependencies supplies repository, transport, metadata, and exit services.
type Dependencies struct {
	ApplySourceConfig func(*search.Params) error
	CurrentRepository func() string
	ServerDefault     func(string) string
	RequestRemote     func(context.Context, api.GritRequest, string) (api.GritResponse, error)
	Metadata          MetadataBuilder
	RequestExit       func(int)
	Stdout            io.Writer
	Stderr            io.Writer
}

type command struct {
	application  cliruntime.Context
	dependencies Dependencies
}

// New constructs the grit command from the common command context.
func New(application cliruntime.Context) cliruntime.Command {
	return &command{application: application}
}

func newWithDependencies(dependencies Dependencies) cliruntime.Command {
	return &command{dependencies: dependencies}
}

func (command *command) services() Dependencies {
	if command.application == nil {
		return command.dependencies
	}
	application := command.application
	return Dependencies{
		ApplySourceConfig: search.SourcePolicyConfigurer(application.Repository()),
		CurrentRepository: application.Repository().Current,
		ServerDefault:     application.Configuration().ServerDefault,
		RequestRemote: func(ctx context.Context, request api.GritRequest, server string) (api.GritResponse, error) {
			return application.APIClient().Grit(ctx, server, request)
		},
		Metadata: func(values Arguments, response api.GritResponse, remote bool) *api.ResultMetadata {
			return gritResultMetadata(application, values, response, remote)
		},
		RequestExit: application.RequestExit,
		Stdout:      application.Stdout(),
		Stderr:      application.Stderr(),
	}
}

func (d Dependencies) applySourceConfig(params *search.Params) error {
	if d.ApplySourceConfig == nil {
		return nil
	}
	return d.ApplySourceConfig(params)
}
func (d Dependencies) currentRepository() string {
	if d.CurrentRepository == nil {
		return ""
	}
	return d.CurrentRepository()
}
func (d Dependencies) serverDefault(value string) string {
	if d.ServerDefault == nil {
		return value
	}
	return d.ServerDefault(value)
}
func (d Dependencies) requestRemote(ctx context.Context, request api.GritRequest, server string) (api.GritResponse, error) {
	if d.RequestRemote == nil {
		return api.GritResponse{}, fmt.Errorf("remote grit transport is unavailable")
	}
	return d.RequestRemote(ctx, request, server)
}
func (d Dependencies) metadata(values gritArgs, response api.GritResponse, remote bool) *api.ResultMetadata {
	if d.Metadata == nil {
		total := response.Total
		complete := len(response.Truncations) == 0 && len(response.ShardErrors) == 0
		return &api.ResultMetadata{Page: api.ResultPage{Skip: values.Skip, Limit: values.Limit, Returned: len(response.Findings), Total: &total, Complete: complete}, Limits: api.ResultLimits{MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSONByteUncapped: values.JSON}}
	}
	return d.Metadata(values, response, remote)
}
func (d Dependencies) requestExit(code int) {
	if d.RequestExit != nil {
		d.RequestExit(code)
	}
}

type outputWriter struct{ output *cliruntime.Output }

func outputDestination(dependencies Dependencies) io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
}
func stdoutWriter(dependencies Dependencies) *outputWriter {
	return &outputWriter{cliruntime.NewOutput(outputDestination(dependencies))}
}
func newBoundedOutputWriter(writer io.Writer, maxBytes int) *outputWriter {
	return &outputWriter{cliruntime.NewBoundedOutput(writer, maxBytes)}
}
func (w *outputWriter) writeString(value string) error { return w.output.WriteString(value) }
func (w *outputWriter) writeJSON(value any) error      { return w.output.WriteJSON(value) }
func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

// Validate validates structural command arguments.
func Validate(values Arguments) error { return validateGritArgs(values) }

// Compile compiles one structural query using command limits.
func Compile(values Arguments) (string, *gritql.Program, error) { return compileGritQuery(values) }

// Request projects command arguments into a remote request.
func Request(values Arguments, query string) api.GritRequest { return gritRequest(values, query) }

// AcquireLocal executes a compiled query against the local checkout.
func AcquireLocal(ctx context.Context, values Arguments, program *gritql.Program, dependencies Dependencies) (api.GritResponse, error) {
	return acquireGritLocal(ctx, values, program, dependencies)
}

// WindowFindings applies a deterministic result window.
func WindowFindings(findings []api.GritFinding, skip, limit int) []api.GritFinding {
	return windowGritFindings(findings, skip, limit)
}
