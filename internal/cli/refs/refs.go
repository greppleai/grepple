package refs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/repositoryrefs"
)

type Args struct {
	cliruntime.CommonArgs
	JSON bool   `arg:"--json" help:"print deterministic JSON"`
	Kind string `arg:"--kind" placeholder:"KIND" help:"only default, branch, or tag refs"`
	Repo string `arg:"positional" placeholder:"OWNER/REPO" help:"only refs for this source repository"`
}

func (Args) Description() string {
	return "List the branches, tags, and resolved commits currently indexed by the remote service."
}

// command owns indexed-ref behavior.
type command struct{ context cliruntime.Context }

// New constructs the refs command.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

// Run executes refs with the common command context.

// Dependencies is retained as a source-compatible alias of the shared context.
type Dependencies = cliruntime.Environment

// Run executes the refs command.
func (command *command) Run(args []string) error {
	application := command.context
	values := Args{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple refs"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(application.Stdout())
			return nil
		}
		return err
	}
	return Execute(application, &values)
}

// Execute lists indexed refs from application-parsed arguments.
func Execute(application cliruntime.Context, values *Args) error {
	values.Kind = strings.ToLower(strings.TrimSpace(values.Kind))
	if values.Kind != "" && values.Kind != "default" && values.Kind != "branch" && values.Kind != "tag" {
		return fmt.Errorf("--kind must be default, branch, or tag")
	}
	server := values.Server
	if configuration := application.Configuration(); configuration != nil {
		server = configuration.ServerDefault(server)
	}
	entries, err := application.APIClient().Repos(context.Background(), server)
	if err != nil {
		return err
	}
	entries = repositoryrefs.Filter(entries, values.Repo, values.Kind)
	if values.JSON {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(api.ReposResponse{OK: true, Count: len(entries), Repos: entries})
	}
	return renderRefs(entries, application)
}

func renderRefs(entries []api.RepoListEntry, application cliruntime.Context) error {
	for _, entry := range entries {
		head := ""
		if entry.Head != nil {
			head = *entry.Head
		}
		selector := entry.Selector
		if selector == "" {
			selector = entry.Repo
		}
		if err := cliruntime.NewOutput(application.Stdout()).WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n", selector, entry.RefKind, entry.Ref, head)); err != nil {
			return err
		}
	}
	if len(entries) == 0 {
		application.RequestExit(1)
	}
	return nil
}

// Filter selects references by source repository and reference kind.
func Filter(entries []api.RepoListEntry, repo, kind string) []api.RepoListEntry {
	return repositoryrefs.Filter(entries, repo, kind)
}
