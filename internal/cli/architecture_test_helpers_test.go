package cli

import architecturecommand "github.com/greppleai/grepple/internal/cli/architecture"

type directoryArchitecture = architecturecommand.Report
type architectureDirectory = architecturecommand.Directory
type architectureRelation = architecturecommand.Relation

func runArchitectureDirectory(args []string) error {
	return runCommand(append([]string{"architecture", "directory"}, args...))
}
func runArchitectureResponsibilities(args []string) error {
	return runCommand(append([]string{"architecture", "responsibilities"}, args...))
}
func buildDirectoryArchitecture(paths []string, maxFiles int) (directoryArchitecture, error) {
	return architecturecommand.Build(paths, maxFiles, architecturecommand.Dependencies{ApplySourceConfig: applyRepositorySourceConfig})
}
func buildArchitectureResponsibilitiesOutput(report directoryArchitecture) architecturecommand.ResponsibilitiesOutput {
	return architecturecommand.BuildResponsibilities(report)
}
