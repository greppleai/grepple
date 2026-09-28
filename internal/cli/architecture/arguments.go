package architecture

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type DirectoryArgs = architectureArgs

type Args struct {
	Directory *DirectoryArgs `arg:"subcommand:directory"`
}

// ExecuteWithDaemon enables the optional local CLI daemon for architecture reports.
func ExecuteWithDaemon(application cliruntime.Context, values *Args, daemon bool) error {
	if values == nil || values.Directory == nil {
		return fmt.Errorf("architecture requires directory")
	}
	dependencies := New(application).(*command).services()
	dependencies.Daemon = daemon
	return executeArchitectureDirectory(values.Directory, dependencies)
}
