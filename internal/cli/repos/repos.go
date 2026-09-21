package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

type reposArgs struct {
	cliruntime.CommonArgs
	JSON   bool   `arg:"--json" help:"print raw JSON"`
	Filter string `arg:"positional" placeholder:"SUBSTRING" help:"only repos whose OWNER/REPO contains this (case-insensitive)"`
}

func (reposArgs) Description() string {
	return "List repositories indexed by the remote shard/router."
}

// command owns repository-listing command dependencies.
type command struct{ dependencies Dependencies }

// New constructs the repos command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

// Run lists repositories. Deprecated: construct the command with New.
func Run(args []string, dependencies Dependencies) error { return New(dependencies).Run(args) }

// Run lists repositories indexed by the configured service.
func (command *command) Run(args []string) error {
	dependencies := command.dependencies
	values := reposArgs{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple repos"}, &values)
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
	base := dependencies.serverDefault(values.Server)
	repos, err := FetchContext(context.Background(), base, dependencies)
	if err != nil {
		return err
	}
	repos = filterRepos(uniqueSourceRepos(repos), values.Filter)
	if values.JSON {
		return cliruntime.NewOutput(dependencies.stdout()).WriteJSON(api.ReposResponse{OK: true, Count: len(repos), Repos: repos})
	}
	output := cliruntime.NewOutput(dependencies.stdout())
	for _, entry := range repos {
		if err := output.WriteString(entry.Repo + "\n"); err != nil {
			return err
		}
	}
	if len(repos) == 0 {
		dependencies.requestExit(1)
	}
	return nil
}

// FetchContext retrieves the server's indexed repository list.
func FetchContext(ctx context.Context, base string, dependencies Dependencies) ([]api.RepoListEntry, error) {
	target := strings.TrimRight(base, "/") + "/public/repos"
	req, err := dependencies.newRequest(http.MethodGet, target, "", nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("server %s returned %d: %s", base, response.StatusCode, string(body))
	}
	var data api.ReposResponse
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Repos, nil
}

func uniqueSourceRepos(repos []api.RepoListEntry) []api.RepoListEntry {
	seen := map[string]bool{}
	unique := make([]api.RepoListEntry, 0, len(repos))
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
func filterRepos(repos []api.RepoListEntry, filter string) []api.RepoListEntry {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return repos
	}
	kept := make([]api.RepoListEntry, 0, len(repos))
	for _, entry := range repos {
		if strings.Contains(strings.ToLower(entry.Repo), filter) {
			kept = append(kept, entry)
		}
	}
	return kept
}
