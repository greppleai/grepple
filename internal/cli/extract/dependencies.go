package extract

import (
	"fmt"
	"io"
	"os"

	codeextract "github.com/greppleai/grepple/extract"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

// Dependencies supplies source discovery and process-owned output.
type Dependencies struct {
	Stdout      io.Writer
	LoadSources func([]string) ([]codeextract.Source, error)
}

type command struct{ dependencies Dependencies }

// New constructs the extract command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

func (d Dependencies) stdout() io.Writer {
	if d.Stdout != nil {
		return d.Stdout
	}
	return os.Stdout
}
func (d Dependencies) loadSources(roots []string) ([]codeextract.Source, error) {
	if d.LoadSources == nil {
		return nil, fmt.Errorf("extract source discovery is unavailable")
	}
	return d.LoadSources(roots)
}
