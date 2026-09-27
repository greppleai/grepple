package repos

import (
	"context"
	"errors"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/wire"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type Args struct {
	cliruntime.CommonArgs
	JSON   bool   `arg:"--json" help:"print raw JSON"`
	Filter string `arg:"positional" placeholder:"SUBSTRING" help:"only repos whose OWNER/REPO contains this (case-insensitive)"`
}

func (Args) Description() string {
	return "List repositories indexed by the remote shard/router."
}

// command owns repository-listing behavior.
type command struct{ context cliruntime.Context }

// New constructs the repos command.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

// Run lists repositories with the common command context.

// Dependencies is retained as a source-compatible alias of the shared context.
type Dependencies = cliruntime.Environment

// Run lists repositories indexed by the configured service.
func (command *command) Run(args []string) error {
	application := command.context
	values := Args{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple repos"}, &values)
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

// Execute lists indexed repositories from application-parsed arguments.
func Execute(application cliruntime.Context, values *Args) error {
	server := values.Server
	if configuration := application.Configuration(); configuration != nil {
		server = configuration.ServerDefault(server)
	}
	repos, err := application.APIClient().Repos(context.Background(), server)
	if err != nil {
		return err
	}
	repos = filterRepos(uniqueSourceRepos(repos), values.Filter)
	if values.JSON {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(wire.ReposResponse{OK: true, Count: len(repos), Repos: repos})
	}
	output := cliruntime.NewOutput(application.Stdout())
	for _, entry := range repos {
		if err := output.WriteString(entry.Repo + "\n"); err != nil {
			return err
		}
	}
	if len(repos) == 0 {
		application.RequestExit(1)
	}
	return nil
}

func uniqueSourceRepos(repos []wire.RepoListEntry) []wire.RepoListEntry {
	seen := map[string]bool{}
	unique := make([]wire.RepoListEntry, 0, len(repos))
	for _, entry := range repos {
		if seen[entry.Repo] {
			continue
		}
		seen[entry.Repo] = true
		unique = append(unique, entry)
	}
	return unique
}

// filterRepos keeps only the repos whose name contains the substring filter
// (case-insensitive); an empty filter keeps everything.
func filterRepos(repos []wire.RepoListEntry, filter string) []wire.RepoListEntry {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return repos
	}
	kept := make([]wire.RepoListEntry, 0, len(repos))
	for _, entry := range repos {
		if strings.Contains(strings.ToLower(entry.Repo), filter) {
			kept = append(kept, entry)
		}
	}
	return kept
}
