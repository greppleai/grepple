package refs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	reposcommand "github.com/greppleai/grepple/internal/cli/repos"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

type refsArgs struct {
	cliruntime.CommonArgs
	JSON bool   `arg:"--json" help:"print deterministic JSON"`
	Kind string `arg:"--kind" placeholder:"KIND" help:"only default, branch, or tag refs"`
	Repo string `arg:"positional" placeholder:"OWNER/REPO" help:"only refs for this source repository"`
}

func (refsArgs) Description() string {
	return "List the branches, tags, and resolved commits currently indexed by the remote service."
}

// command owns indexed-ref command dependencies.
type command struct{ dependencies Dependencies }

// New constructs the refs command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

// Run executes the refs command. Deprecated: construct the command with New.
func Run(args []string, dependencies Dependencies) error { return New(dependencies).Run(args) }

// Run executes the refs command.
func (command *command) Run(args []string) error {
	dependencies := command.dependencies
	values := refsArgs{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple refs"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(dependencies.stdout())
			return nil
		}
		return err
	}
	values.Kind = strings.ToLower(strings.TrimSpace(values.Kind))
	if values.Kind != "" && values.Kind != "default" && values.Kind != "branch" && values.Kind != "tag" {
		return fmt.Errorf("--kind must be default, branch, or tag")
	}
	entries, err := reposcommand.FetchContext(context.Background(), dependencies.serverDefault(values.Server), reposcommand.Dependencies{NewRequest: dependencies.NewRequest})
	if err != nil {
		return err
	}
	entries = Filter(entries, values.Repo, values.Kind)
	if values.JSON {
		return cliruntime.NewOutput(dependencies.stdout()).WriteJSON(api.ReposResponse{OK: true, Count: len(entries), Repos: entries})
	}
	return renderRefs(entries, dependencies)
}

func renderRefs(entries []api.RepoListEntry, dependencies Dependencies) error {
	for _, entry := range entries {
		head := ""
		if entry.Head != nil {
			head = *entry.Head
		}
		selector := entry.Selector
		if selector == "" {
			selector = entry.Repo
		}
		if err := cliruntime.NewOutput(dependencies.stdout()).WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n", selector, entry.RefKind, entry.Ref, head)); err != nil {
			return err
		}
	}
	if len(entries) == 0 {
		dependencies.requestExit(1)
	}
	return nil
}

// Filter selects references by source repository and reference kind.
func Filter(entries []api.RepoListEntry, repo, kind string) []api.RepoListEntry {
	repo = strings.TrimSpace(repo)
	kind = strings.TrimSpace(strings.ToLower(kind))
	if repo == "" && kind == "" {
		return entries
	}
	kept := make([]api.RepoListEntry, 0, len(entries))
	for _, entry := range entries {
		if (repo == "" || entry.Repo == repo) && (kind == "" || entry.RefKind == kind) {
			kept = append(kept, entry)
		}
	}
	return kept
}
