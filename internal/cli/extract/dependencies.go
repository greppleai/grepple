package extract

import (
	"fmt"
	"io"
	"os"

	codeextract "github.com/greppleai/grepple/extract"
)

// Dependencies supplies source discovery and process-owned output.
type Dependencies struct {
	Stdout      io.Writer
	LoadSources func([]string) ([]codeextract.Source, error)
}

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
