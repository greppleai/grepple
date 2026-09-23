package grit

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type ExplainArgs = gritExplainArgs

type Args struct {
	Run     *Arguments   `arg:"subcommand:run"`
	Explain *ExplainArgs `arg:"subcommand:explain"`
}

func Execute(application cliruntime.Context, values *Args) error {
	if values == nil {
		return fmt.Errorf("grit arguments are required")
	}
	command := New(application).(*command)
	if values.Explain != nil {
		return executeGritExplain(values.Explain, command.services())
	}
	if values.Run == nil {
		return fmt.Errorf("grit requires run or explain")
	}
	return command.execute(values.Run)
}
