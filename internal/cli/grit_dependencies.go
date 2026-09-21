package cli

import (
	gritcommand "github.com/greppleai/grepple/internal/cli/grit"
	"github.com/greppleai/grepple/internal/gitcontext"
)

func gritDependencies() gritcommand.Dependencies {
	return gritcommand.Dependencies{ApplySourceConfig: applyRepositorySourceConfig, CurrentRepository: gitcontext.Current, ServerDefault: serverDefault, RequestRemote: requestGritRemote, Metadata: gritResultMetadata, RequestExit: setExit}
}
