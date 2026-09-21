package cli

import rulescommand "github.com/greppleai/grepple/internal/cli/rules"

func runRules(args []string) error {
	return rulescommand.New(rulescommand.Dependencies{ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit}).Run(args)
}
