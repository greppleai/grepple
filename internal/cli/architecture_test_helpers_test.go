package cli

import (
	"os"

	architecturecommand "github.com/greppleai/grepple/internal/cli/architecture"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/search"
)

type directoryArchitecture = architecturecommand.Report
type architectureDirectory = architecturecommand.Directory
type architectureRelation = architecturecommand.Relation

func buildDirectoryArchitecture(paths []string, maxFiles int) (directoryArchitecture, error) {
	return architecturecommand.Build(paths, maxFiles, architecturecommand.Dependencies{ApplySourceConfig: search.SourcePolicyConfigurer(cliruntime.NewRepository(cliruntime.RepositoryInvocationOptions{}, os.Stderr))})
}
func buildArchitectureResponsibilitiesOutput(report directoryArchitecture) architecturecommand.ResponsibilitiesOutput {
	return architecturecommand.BuildResponsibilities(report)
}
