package cli

import gritcommand "github.com/greppleai/grepple/internal/cli/grit"

func gritDependencies() gritcommand.Dependencies {
	return gritcommand.Dependencies{ApplySourceConfig: applyRepositorySourceConfig, CurrentRepository: currentGitRepoID, ServerDefault: serverDefault, RequestRemote: requestGritRemote, Metadata: gritResultMetadata, RequestExit: setExit}
}

func runGrit(args []string) error {
	return gritcommand.New(gritDependencies()).Run(args)
}
