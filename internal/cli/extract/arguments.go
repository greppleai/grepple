package extract

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type GenerateArgs = extractArgs

type CheckModeArgs struct {
	Target  string   `arg:"positional,required" placeholder:"TARGET"`
	Sources []string `arg:"positional" placeholder:"SOURCE"`
}

type CheckArgs struct {
	Structure *CheckModeArgs `arg:"subcommand:structure"`
	Flow      *CheckModeArgs `arg:"subcommand:flow"`
}

type Args struct {
	Structure *GenerateArgs `arg:"subcommand:structure"`
	Flow      *GenerateArgs `arg:"subcommand:flow"`
	Check     *CheckArgs    `arg:"subcommand:check"`
}

func Execute(application cliruntime.Context, values *Args) error {
	if values == nil {
		return extractUsageError()
	}
	command := New(application).(*command)
	switch {
	case values.Structure != nil:
		if err := validateExtractArgs(values.Structure); err != nil {
			return err
		}
		return command.runStructure(values.Structure)
	case values.Flow != nil:
		if err := validateExtractArgs(values.Flow); err != nil {
			return err
		}
		return command.runFlow(values.Flow)
	case values.Check != nil && values.Check.Structure != nil:
		return checkExtractDiagram(values.Check.Structure.Target, values.Check.Structure.Sources, true)
	case values.Check != nil && values.Check.Flow != nil:
		return checkExtractDiagram(values.Check.Flow.Target, values.Check.Flow.Sources, false)
	default:
		return fmt.Errorf("extract requires structure, flow, or check")
	}
}
