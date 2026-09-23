// Package verify checks repository-owned directory metadata.
package verify

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type Args struct {
	JSON  bool     `arg:"--json" help:"emit a structured verification report"`
	Paths []string `arg:"positional" placeholder:"PATH" help:"source path or glob; defaults to the repository"`
}

type Report struct {
	Schema      string   `json:"schema"`
	Root        string   `json:"root"`
	Directories int      `json:"directories"`
	Valid       int      `json:"valid"`
	Missing     []string `json:"missing,omitempty"`
	Stale       []string `json:"stale,omitempty"`
	Invalid     []string `json:"invalid,omitempty"`
}

type command struct{ context cliruntime.Context }

func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

func (command *command) Run(args []string) error {
	values := Args{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple verify"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(command.context.Stdout())
			return nil
		}
		return err
	}
	return Execute(command.context, &values)
}

// Execute verifies metadata from application-parsed arguments.
func Execute(application cliruntime.Context, values *Args) error {
	report, err := Build(application, values.Paths)
	if err != nil {
		return err
	}
	if values.JSON {
		if err := cliruntime.NewOutput(application.Stdout()).WriteJSON(report); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(application.Stdout(), "verify directories=%d valid=%d missing=%d stale=%d invalid=%d\n", report.Directories, report.Valid, len(report.Missing), len(report.Stale), len(report.Invalid))
		for _, path := range report.Missing {
			fmt.Fprintln(application.Stdout(), "M", path)
		}
		for _, path := range report.Stale {
			fmt.Fprintln(application.Stdout(), "S", path)
		}
		for _, path := range report.Invalid {
			fmt.Fprintln(application.Stdout(), "I", path)
		}
	}
	if len(report.Missing) > 0 || len(report.Stale) > 0 || len(report.Invalid) > 0 {
		application.RequestExit(1)
	}
	return nil
}

func Build(context cliruntime.Context, paths []string) (Report, error) {
	root := context.Repository().WorkingDirectory()
	policy, err := context.Repository().ScopeOptions()
	if err != nil {
		return Report{}, err
	}
	files, err := sourcedomain.ListWithPolicy(nil, paths, root, policy)
	if err != nil {
		return Report{}, err
	}
	directories, err := directorymeta.Directories(root, files)
	if err != nil {
		return Report{}, err
	}
	report := Report{Schema: "grepple-directory-metadata-verification-v1", Root: filepath.ToSlash(root), Directories: len(directories), Missing: []string{}, Stale: []string{}, Invalid: []string{}}
	for _, directory := range directories {
		display, _ := filepath.Rel(root, directory)
		display = filepath.ToSlash(display)
		inspection := directorymeta.Inspect(root, directory, files)
		switch inspection.Status {
		case directorymeta.StatusMissing:
			report.Missing = append(report.Missing, display)
		case directorymeta.StatusStale:
			report.Stale = append(report.Stale, display)
		case directorymeta.StatusInvalid:
			report.Invalid = append(report.Invalid, display)
		default:
			report.Valid++
		}
	}
	return report, nil
}
