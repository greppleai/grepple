package artifacts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexflint/go-arg"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type artifactsCleanArgs struct {
	JSON bool `arg:"--json" help:"emit cleanup totals as JSON"`
}

// CleanOutput describes one artifact cleanup result.
type CleanOutput struct {
	Schema string `json:"schema"`
	Path   string `json:"path"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}

// command owns artifact command dependencies.
type command struct{ dependencies Dependencies }

// New constructs the artifacts command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

// Run executes the artifacts command. Deprecated: construct the command with New.
func Run(args []string, dependencies Dependencies) error { return New(dependencies).Run(args) }

// Run executes the artifacts command.
func (command *command) Run(args []string) error {
	dependencies := command.dependencies
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return cliruntime.NewOutput(dependencies.stdout()).WriteString("Manage content-addressed command output artifacts.\nUsage:\n  grepple artifacts clean [--json]\n")
	}
	if args[0] != "clean" {
		return fmt.Errorf("usage: grepple artifacts clean [--json]")
	}
	values := artifactsCleanArgs{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple artifacts clean"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args[1:]); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(dependencies.stdout())
			return nil
		}
		return err
	}
	return cleanOutputArtifacts(values.JSON, dependencies)
}

func cleanOutputArtifacts(jsonMode bool, dependencies Dependencies) error {
	directory, err := dependencies.artifactDirectory()
	if err != nil {
		return err
	}
	result := CleanOutput{Schema: "grepple-artifact-clean-v1", Path: displayArtifactPath(directory, dependencies)}
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
		return cliruntime.NewOutput(dependencies.stdout()).WriteJSON(result)
	}
	return cliruntime.NewOutput(dependencies.stdout()).WriteString(fmt.Sprintf("removed artifacts path=%s files=%d bytes=%d\n", result.Path, result.Files, result.Bytes))
}

func displayArtifactPath(path string, dependencies Dependencies) string {
	relative, err := filepath.Rel(dependencies.workingDirectory(), path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
