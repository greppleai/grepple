package anchors

import (
	"io"
	"os"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

type cliOptions struct {
	Anchors     bool
	LineOnly    bool
	Params      search.Params
	AnchorLines Lookup
}

func prepareResultAnchors(options *cliOptions, results []api.FileResult) error {
	lookup, err := Prepare(&SearchOptions{Enabled: options.Anchors, LineOnly: options.LineOnly, Params: options.Params}, results)
	options.AnchorLines = lookup
	return err
}
func Run(args []string) error {
	if len(args) >= 2 && args[0] == "anchors" {
		return New(Dependencies{}).Run(args[1:])
	}
	if len(args) >= 3 && args[0] == "help" && args[1] == "anchors" {
		switch args[2] {
		case "doctor":
			return RunDoctor([]string{"--help"})
		case "setup":
			return RunSetup([]string{"--help"})
		}
	}
	return New(Dependencies{}).Run(args)
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = previous }()
	run()
	_ = writer.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	return string(content)
}
