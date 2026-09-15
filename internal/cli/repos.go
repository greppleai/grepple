package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/greppleai/grepple/api"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/alexflint/go-arg"
)

type reposArgs struct {
	Server string `arg:"-s,--server" placeholder:"URL" help:"remote shard/router URL"`
	JSON   bool   `arg:"--json" help:"print raw JSON"`
	Filter string `arg:"positional" placeholder:"SUBSTRING" help:"only repos whose OWNER/REPO contains this (case-insensitive)"`
}

func (reposArgs) Description() string {
	return "List repositories indexed by the remote shard/router."
}

// runRepos implements `grepple repos`: list the repositories a server has indexed,
// so callers can discover the exact OWNER/REPO name to use with get/tree/--repo
// without probing via a throwaway --count search.
func runRepos(args []string) error {
	values := reposArgs{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple repos"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	base := serverDefault(values.Server)
	repos, err := fetchRepoList(base)
	if err != nil {
		return err
	}
	repos = filterRepos(uniqueSourceRepos(repos), values.Filter)
	if values.JSON {
		return stdoutWriter().writeJSON(api.ReposResponse{OK: true, Count: len(repos), Repos: repos})
	}
	for _, entry := range repos {
		if err := stdoutWriter().writeString(entry.Repo + "\n"); err != nil {
			return err
		}
	}
	if len(repos) == 0 {
		setExit(1)
	}
	return nil
}

// fetchRepoList retrieves the server's indexed repository list, surfacing the
// server's error text for non-2xx responses.
func fetchRepoList(base string) ([]api.RepoListEntry, error) {
	target := strings.TrimRight(base, "/") + "/public/repos"
	req, err := authorizedRequest(http.MethodGet, target, "", nil)
	if err != nil {
		return nil, err
	}
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
