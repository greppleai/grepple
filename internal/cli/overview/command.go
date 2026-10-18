// Package overview summarizes trustworthy file annotations in a local repository.
package overview

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

// Limit the output independent of repository size, including JSON. Counts remain complete.
const maxLabels = 8
const maxExamples = 2

type Args struct {
	JSON bool `arg:"--json" help:"emit a structured, bounded metadata overview"`
}

type Coverage struct {
	Selected int `json:"selected"`
	Current  int `json:"current"`
	Stale    int `json:"stale"`
	Missing  int `json:"missing"`
	Invalid  int `json:"invalid"`
}

type Label struct {
	Name         string   `json:"name"`
	Files        int      `json:"files"`
	Paths        []string `json:"paths"`
	OmittedPaths int      `json:"omittedPaths"`
}

type Category struct {
	Total     int     `json:"total"` // distinct labels, including those not displayed
	Shown     []Label `json:"shown"`
	Truncated int     `json:"truncated"`
}

type Report struct {
	Schema          string   `json:"schema"`
	Root            string   `json:"root"`
	Files           Coverage `json:"files"`
	Directories     Coverage `json:"directories"`
	Purposes        Category `json:"purposes"`
	DomainAreas     Category `json:"domainAreas"`
	FunctionalAreas Category `json:"functionalAreas"`
	Keywords        Category `json:"keywords"`
	Examples        []string `json:"examples"`
	Guidance        []string `json:"guidance"`
}

func Execute(application cliruntime.Context, args *Args) error {
	report, err := Build(application)
	if err != nil {
		return err
	}
	if args.JSON {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(report)
	}
	_, err = fmt.Fprint(application.Stdout(), report.Text())
	return err
}

// Build uses precisely the same source-scoped V1 inventory as tree and verify.
// Reading YAML never causes source selection or a model invocation.
func Build(application cliruntime.Context) (Report, error) {
	root := application.Repository().WorkingDirectory()
	policy, err := application.Repository().ScopeOptions()
	if err != nil {
		return Report{}, err
	}
	paths, err := sourcedomain.ListWithPolicy(nil, nil, root, policy)
	if err != nil {
		return Report{}, err
	}
	inventory, err := directorymeta.BuildInventoryV1(root, paths)
	if err != nil {
		return Report{}, err
	}
	report := Report{Schema: "grepple-overview-v1", Root: filepath.ToSlash(root), Examples: []string{}, Guidance: []string{}}
	for _, state := range inventory.Files {
		addStatus(&report.Files, state.Status)
	}
	report.Files.Selected = len(inventory.Files)
	for _, inspection := range inventory.Directories {
		addStatus(&report.Directories, inspection.Status)
	}
	report.Directories.Selected = len(inventory.Directories)
	purpose, domain, functional, keyword := newCategory(), newCategory(), newCategory(), newCategory()
	keys := make([]string, 0, len(inventory.Files))
	for path := range inventory.Files {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		state := inventory.Files[path]
		if state.Status != directorymeta.StatusCurrent {
			continue
		}
		purpose.add(state.File.Purpose, path)
		for _, label := range state.File.DomainAreas {
			domain.add(label, path)
		}
		for _, label := range state.File.FunctionalAreas {
			functional.add(label, path)
		}
		for _, label := range state.File.Keywords {
			keyword.add(label, path)
		}
	}
	report.Purposes, report.DomainAreas = purpose.finish(), domain.finish()
	report.FunctionalAreas, report.Keywords = functional.finish(), keyword.finish()
	if report.Purposes.Total > 0 {
		report.Examples = append(report.Examples, "grepple tree --purpose "+quoteLabel(report.Purposes.Shown[0].Name)+" --depth 2")
	}
	if report.DomainAreas.Total > 0 {
		report.Examples = append(report.Examples, "grepple search --domain-area "+quoteLabel(report.DomainAreas.Shown[0].Name)+" 'PATTERN'")
	} else if report.Keywords.Total > 0 {
		report.Examples = append(report.Examples, "grepple search --keyword "+quoteLabel(report.Keywords.Shown[0].Name)+" 'PATTERN'")
	}
	if report.Files.Selected == 0 {
		report.Guidance = append(report.Guidance, "No selected files; check source paths and ignore/production-only policy.")
	}
	if report.Files.Missing+report.Files.Stale+report.Files.Invalid > 0 || report.Directories.Missing+report.Directories.Stale+report.Directories.Invalid > 0 {
		report.Guidance = append(report.Guidance, "Run grepple verify to inspect coverage; run grepple init to annotate missing or changed files (model generation is explicit).")
	}
	return report, nil
}

func addStatus(count *Coverage, status string) {
	switch status {
	case directorymeta.StatusCurrent:
		count.Current++
	case directorymeta.StatusStale:
		count.Stale++
	case directorymeta.StatusMissing:
		count.Missing++
	default:
		count.Invalid++
	}
}

type categoryBuilder struct{ values map[string]*Label }

func newCategory() categoryBuilder { return categoryBuilder{values: map[string]*Label{}} }
func (builder categoryBuilder) add(value, path string) {
	key := directorymeta.LabelKey(value)
	label := builder.values[key]
	if label == nil {
		label = &Label{Name: value, Paths: []string{}}
		builder.values[key] = label
	}
	label.Files++
	if len(label.Paths) < maxExamples {
		label.Paths = append(label.Paths, path)
	}
}
func (builder categoryBuilder) finish() Category {
	all := make([]Label, 0, len(builder.values))
	for _, value := range builder.values {
		value.OmittedPaths = value.Files - len(value.Paths)
		all = append(all, *value)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Files != all[j].Files {
			return all[i].Files > all[j].Files
		}
		return directorymeta.LabelKey(all[i].Name) < directorymeta.LabelKey(all[j].Name)
	})
	result := Category{Total: len(all), Shown: all}
	if len(all) > maxLabels {
		result.Shown = all[:maxLabels]
		result.Truncated = len(all) - maxLabels
	}
	return result
}

func (report Report) Text() string {
	var out strings.Builder
	fmt.Fprintf(&out, "overview: %d/%d current files (%d stale, %d missing, %d invalid); %d/%d current directories\n", report.Files.Current, report.Files.Selected, report.Files.Stale, report.Files.Missing, report.Files.Invalid, report.Directories.Current, report.Directories.Selected)
	for _, section := range []struct {
		name     string
		category Category
	}{
		{"purposes", report.Purposes}, {"domain areas", report.DomainAreas}, {"functional areas", report.FunctionalAreas}, {"keywords", report.Keywords},
	} {
		fmt.Fprintf(&out, "%s (%d distinct", section.name, section.category.Total)
		if section.category.Truncated > 0 {
			fmt.Fprintf(&out, ", %d omitted", section.category.Truncated)
		}
		fmt.Fprintln(&out, "):")
		for _, label := range section.category.Shown {
			fmt.Fprintf(&out, "  %s (%d files): %s", label.Name, label.Files, strings.Join(label.Paths, ", "))
			if label.OmittedPaths > 0 {
				fmt.Fprintf(&out, " (+%d paths)", label.OmittedPaths)
			}
			fmt.Fprintln(&out)
		}
	}
	for _, example := range report.Examples {
		fmt.Fprintln(&out, example)
	}
	for _, guidance := range report.Guidance {
		fmt.Fprintln(&out, guidance)
	}
	return out.String()
}

// Always quote metadata-supplied shell arguments, even without whitespace:
// validated labels may still contain shell metacharacters.
func quoteLabel(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
