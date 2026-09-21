package cli

import (
	"os"
	"path/filepath"

	codeextract "github.com/greppleai/grepple/extract"
	extractcommand "github.com/greppleai/grepple/internal/cli/extract"
)

func extractDependencies() extractcommand.Dependencies {
	return extractcommand.Dependencies{Stdout: os.Stdout, LoadSources: loadExtractCommandSources}
}
func runExtract(args []string) error              { return extractcommand.New(extractDependencies()).Run(args) }
func extractAt(value string) (string, int, error) { return extractcommand.ParseAt(value) }

func loadExtractCommandSources(roots []string) ([]codeextract.Source, error) {
	config, path, err := loadRepositoryConfig()
	if err != nil {
		return nil, err
	}
	options := codeextract.DiscoveryOptions{IgnoreRoot: mustGetwd(), ProductionOnly: activeRepositoryOptions.productionOnly}
	if path != "" && !activeRepositoryOptions.ignoreDisabled {
		options.IgnoreRoot = filepath.Dir(path)
		options.IgnorePaths = append([]string(nil), config.Ignore.Paths...)
	}
	reportExplicitSourceBypasses(roots, options.IgnoreRoot, options.IgnorePaths, options.ProductionOnly)
	return codeextract.LoadSourcesWithOptions(roots, options)
}
