package cli

import (
	codeextract "github.com/greppleai/grepple/extract"
	extractcommand "github.com/greppleai/grepple/internal/cli/extract"
	"github.com/greppleai/grepple/internal/repositoryscope"
	"os"
)

func extractDependencies() extractcommand.Dependencies {
	return extractcommand.Dependencies{Stdout: os.Stdout, LoadSources: loadExtractCommandSources}
}
func runExtract(args []string) error              { return extractcommand.New(extractDependencies()).Run(args) }
func extractAt(value string) (string, int, error) { return extractcommand.ParseAt(value) }

func loadExtractCommandSources(roots []string) ([]codeextract.Source, error) {
	options, err := repositoryScopeOptions()
	if err != nil {
		return nil, err
	}
	return repositoryscope.LoadSources(roots, options)
}
