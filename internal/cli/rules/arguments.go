package rules

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type AddArgs = rulesAddArgs

type ServerArgs struct {
	commonArgs
	JSON bool `arg:"--json"`
}

type IDArgs struct {
	commonArgs
	JSON bool   `arg:"--json"`
	ID   string `arg:"positional,required" placeholder:"ID"`
}

type Args struct {
	Add     *AddArgs    `arg:"subcommand:add"`
	Create  *AddArgs    `arg:"subcommand:create"`
	List    *ServerArgs `arg:"subcommand:list"`
	LS      *ServerArgs `arg:"subcommand:ls"`
	Get     *IDArgs     `arg:"subcommand:get"`
	Show    *IDArgs     `arg:"subcommand:show"`
	RM      *IDArgs     `arg:"subcommand:rm"`
	Remove  *IDArgs     `arg:"subcommand:remove"`
	Delete  *IDArgs     `arg:"subcommand:delete"`
	Results *IDArgs     `arg:"subcommand:results"`
}

func Execute(application cliruntime.Context, values *Args) error {
	if values == nil {
		return fmt.Errorf("rules arguments are required")
	}
	dependencies := New(application).(*command).dependencies
	switch {
	case values.Add != nil:
		return executeRulesAdd(values.Add, dependencies)
	case values.Create != nil:
		return executeRulesAdd(values.Create, dependencies)
	case values.List != nil:
		return executeRulesList(values.List, dependencies)
	case values.LS != nil:
		return executeRulesList(values.LS, dependencies)
	case values.Get != nil:
		return executeRulesGet(values.Get, dependencies)
	case values.Show != nil:
		return executeRulesGet(values.Show, dependencies)
	case values.RM != nil:
		return executeRulesDelete(values.RM, dependencies)
	case values.Remove != nil:
		return executeRulesDelete(values.Remove, dependencies)
	case values.Delete != nil:
		return executeRulesDelete(values.Delete, dependencies)
	case values.Results != nil:
		return executeRulesResults(values.Results, dependencies)
	default:
		return fmt.Errorf("rules requires add, list, get, rm, or results")
	}
}
