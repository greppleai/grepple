package extract

import (
	"fmt"
	"io"

	codeextract "github.com/greppleai/grepple/extract"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type dependencies struct{ cliruntime.Context }

type command struct{ dependencies dependencies }

// New constructs the extract command.
func New(context cliruntime.Context) cliruntime.Command {
	return &command{dependencies: dependencies{Context: context}}
}

func (dependencies dependencies) stdout() io.Writer { return dependencies.Stdout() }
func (dependencies dependencies) loadSources(roots []string) ([]codeextract.Source, error) {
	repository := dependencies.Repository()
	if repository == nil {
		return nil, fmt.Errorf("extract source discovery is unavailable")
	}
	options, err := repository.ScopeOptions()
	if err != nil {
		return nil, err
	}
	return codeextract.LoadSourcesWithPolicy(roots, options)
}
