package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexflint/go-arg"
)

type artifactsCleanArgs struct {
	JSON bool `arg:"--json" help:"emit cleanup totals as JSON"`
}

type artifactsCleanOutput struct {
	Schema string `json:"schema"`
	Path   string `json:"path"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}

func runArtifacts(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return stdoutWriter().writeString("Manage content-addressed command output artifacts.\nUsage:\n  grepple artifacts clean [--json]\n")
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
			parser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	return cleanOutputArtifacts(values.JSON)
}

func cleanOutputArtifacts(jsonMode bool) error {
	directory, err := defaultOutputArtifactDirectory()
	if err != nil {
		return err
	}
	result := artifactsCleanOutput{Schema: "grepple-artifact-clean-v1", Path: displayArtifactPath(directory)}
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
		return stdoutWriter().writeJSON(result)
	}
	return stdoutWriter().writeString(fmt.Sprintf("removed artifacts path=%s files=%d bytes=%d\n", result.Path, result.Files, result.Bytes))
}

func displayArtifactPath(path string) string {
	relative, err := filepath.Rel(mustGetwd(), path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
