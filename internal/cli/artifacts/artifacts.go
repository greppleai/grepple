package artifacts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexflint/go-arg"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/storagepaths"
)

type CleanArgs struct {
	JSON bool `arg:"--json" help:"emit cleanup totals as JSON"`
}

// Args contains artifact command subcommands.
type Args struct {
	Clean *CleanArgs `arg:"subcommand:clean"`
}

// CleanOutput describes one artifact cleanup result.
type CleanOutput struct {
	Schema string `json:"schema"`
	Path   string `json:"path"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}

// command owns artifact command behavior.
type command struct{ context cliruntime.Context }

// New constructs the artifacts command.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

// Run executes the artifacts command.
func (command *command) Run(args []string) error {
	application := command.context
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return cliruntime.NewOutput(application.Stdout()).WriteString("Manage content-addressed command output artifacts.\nUsage:\n  grepple artifacts clean [--json]\n")
	}
	if args[0] != "clean" {
		return fmt.Errorf("usage: grepple artifacts clean [--json]")
	}
	values := CleanArgs{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple artifacts clean"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args[1:]); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(application.Stdout())
			return nil
		}
		return err
	}
	return Execute(application, &Args{Clean: &values})
}

// Execute applies an application-parsed artifact command.
func Execute(application cliruntime.Context, values *Args) error {
	if values == nil || values.Clean == nil {
		return fmt.Errorf("usage: grepple artifacts clean [--json]")
	}
	return cleanOutputArtifacts(values.Clean.JSON, application)
}

func cleanOutputArtifacts(jsonMode bool, application cliruntime.Context) error {
	directory, err := storagepaths.OutputArtifacts()
	if err != nil {
		return err
	}
	result := CleanOutput{Schema: "grepple-artifact-clean-v1", Path: displayArtifactPath(directory, application)}
	entries, err := os.ReadDir(directory)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if info, statErr := entry.Info(); statErr == nil {
			result.Bytes += info.Size()
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		result.Files++
	}
	if jsonMode {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(result)
	}
	return cliruntime.NewOutput(application.Stdout()).WriteString(fmt.Sprintf("removed artifacts path=%s files=%d bytes=%d\n", result.Path, result.Files, result.Bytes))
}

func displayArtifactPath(path string, application cliruntime.Context) string {
	workingDirectory := "."
	if repository := application.Repository(); repository != nil {
		workingDirectory = repository.WorkingDirectory()
	}
	relative, err := filepath.Rel(workingDirectory, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
