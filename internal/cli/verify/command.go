// Package verify checks repository-owned directory metadata.
package verify

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type Args struct {
	JSON  bool     `arg:"--json" help:"emit a structured verification report"`
	Paths []string `arg:"positional" placeholder:"PATH" help:"source path or glob; defaults to the repository"`
}

type Report struct {
	Schema      string              `json:"schema"`
	Root        string              `json:"root"`
	Directories int                 `json:"directories"`
	Valid       int                 `json:"valid"`
	Missing     []string            `json:"missing,omitempty"`
	Stale       []string            `json:"stale,omitempty"`
	Invalid     []string            `json:"invalid,omitempty"`
	Issues      map[string][]string `json:"issues,omitempty"` // directory-relative path to inspection reasons
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
			printIssues(application, report.Issues[path])
		}
		for _, path := range report.Stale {
			fmt.Fprintln(application.Stdout(), "S", path)
			printIssues(application, report.Issues[path])
		}
		for _, path := range report.Invalid {
			fmt.Fprintln(application.Stdout(), "I", path)
			printIssues(application, report.Issues[path])
		}
	}
	if len(report.Missing) > 0 || len(report.Stale) > 0 || len(report.Invalid) > 0 {
		application.RequestExit(1)
	}
	return nil
}

func printIssues(application cliruntime.Context, issues []string) {
	for _, issue := range issues {
		fmt.Fprintln(application.Stdout(), "  -", issue)
	}
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
	inventory, err := directorymeta.BuildInventoryV1(root, files)
	if err != nil {
		return Report{}, err
	}
	report := Report{Schema: "grepple-metadata-verification-v1", Root: filepath.ToSlash(root), Directories: len(inventory.Directories), Missing: []string{}, Stale: []string{}, Invalid: []string{}, Issues: map[string][]string{}}
	for path, inspection := range inventory.Directories {
		if len(inspection.Issues) != 0 {
			report.Issues[path] = append([]string(nil), inspection.Issues...)
		}
		switch inspection.Status {
		case directorymeta.StatusMissing:
			report.Missing = append(report.Missing, path)
		case directorymeta.StatusStale:
			report.Stale = append(report.Stale, path)
		case directorymeta.StatusInvalid:
			report.Invalid = append(report.Invalid, path)
		default:
			report.Valid++
		}
	}
	sort.Strings(report.Missing)
	sort.Strings(report.Stale)
	sort.Strings(report.Invalid)
	return report, nil
}
