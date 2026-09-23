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
	Directory        *DirectoryArgs        `arg:"subcommand:directory"`
	Resolve          *ResolveArgs          `arg:"subcommand:resolve"`
	Why              *WhyArgs              `arg:"subcommand:why"`
	Responsibilities *ResponsibilitiesArgs `arg:"subcommand:responsibilities"`
	Compare          *CompareArgs          `arg:"subcommand:compare"`
}

func DefaultArgs() Args { return Args{} }

func Execute(application cliruntime.Context, values *Args) error {
	if values == nil {
		return fmt.Errorf("architecture requires directory, resolve, why, responsibilities, or compare")
	}
	dependencies := New(application).(*command).services()
	switch {
	case values.Directory != nil:
		return executeArchitectureDirectory(values.Directory, dependencies)
	case values.Resolve != nil:
		return executeArchitectureResolve(values.Resolve, dependencies)
	case values.Why != nil:
		return executeArchitectureWhy(values.Why, dependencies)
	case values.Responsibilities != nil:
		return executeArchitectureResponsibilities(values.Responsibilities, dependencies)
	case values.Compare != nil:
		return executeArchitectureCompare(values.Compare, dependencies)
	default:
		return fmt.Errorf("architecture requires directory, resolve, why, responsibilities, or compare")
	}
}
