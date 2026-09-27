package architecture

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type DirectoryArgs = architectureArgs
type ResolveArgs = architectureResolveArgs
type WhyArgs = architectureWhyArgs
type ResponsibilitiesArgs = architectureResponsibilitiesArgs
type CompareArgs = architectureCompareArgs

type Args struct {
	Directory *DirectoryArgs `arg:"subcommand:directory"`
}

func DefaultArgs() Args { return Args{} }

func Execute(application cliruntime.Context, values *Args) error {
	return ExecuteWithDaemon(application, values, false)
}

// ExecuteWithDaemon enables the optional local architecture worker for this invocation.
func ExecuteWithDaemon(application cliruntime.Context, values *Args, daemon bool) error {
	if values == nil {
		return fmt.Errorf("architecture requires directory")
	}
	dependencies := New(application).(*command).services()
	dependencies.Daemon = daemon
	switch {
	case values.Directory != nil:
		return executeArchitectureDirectory(values.Directory, dependencies)
	default:
		return fmt.Errorf("architecture requires directory")
	}
}
