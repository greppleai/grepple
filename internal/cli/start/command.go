// Package start provides deterministic, metadata-backed entrypoints to an area.
package start

import (
	"fmt"
	"path/filepath"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type Args struct {
	Area  string   `arg:"--area,required" help:"repository-owned area tag to discover"`
	JSON  bool     `arg:"--json" help:"emit a structured area entrypoint report"`
	Paths []string `arg:"positional" placeholder:"PATH" help:"source path or glob; defaults to the repository"`
}

type Report struct {
	Schema     string                        `json:"schema"`
	Area       string                        `json:"area"`
	References []directorymeta.AreaReference `json:"references"`
	Review     []directorymeta.AreaReference `json:"review"`
}

// Build never invokes a model or guesses membership from text or call edges.
func Build(application cliruntime.Context, values *Args) (Report, error) {
	if !directorymeta.ValidArea(values.Area) {
		return Report{}, fmt.Errorf("invalid --area %q (use lowercase letters, digits and single hyphens)", values.Area)
	}
	root := application.Repository().WorkingDirectory()
	policy, err := application.Repository().ScopeOptions()
	if err != nil {
		return Report{}, err
	}
	files, err := sourcedomain.ListWithPolicy(nil, values.Paths, root, policy)
	if err != nil {
		return Report{}, err
	}
	refs, err := directorymeta.AreaIndex(root, files)
	if err != nil {
		return Report{}, err
	}
	report := Report{Schema: "grepple-area-start-v1", Area: values.Area, References: []directorymeta.AreaReference{}, Review: []directorymeta.AreaReference{}}
	for _, ref := range refs {
		if ref.Area != values.Area {
			continue
		}
		if ref.Status == directorymeta.StatusCurrent {
			report.References = append(report.References, ref)
		} else {
			report.Review = append(report.Review, ref)
		}
	}
	return report, nil
}

func Execute(application cliruntime.Context, values *Args) error {
	report, err := Build(application, values)
	if err != nil {
		return err
	}
	if values.JSON {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(report)
	}
	fmt.Fprintf(application.Stdout(), "start area=%s sources=%d review=%d\n", report.Area, len(report.References), len(report.Review))
	for _, ref := range report.References {
		role := "source"
		if ref.Kind == "test" {
			role = "test"
		}
		fmt.Fprintf(application.Stdout(), "%s %s\n", role, filepath.ToSlash(ref.Path))
	}
	for _, ref := range report.Review {
		fmt.Fprintf(application.Stdout(), "review %s (%s: %s)\n", ref.Path, ref.Status, ref.Issues)
	}
	return nil
}
