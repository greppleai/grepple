package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
)

type refsArgs struct {
	commonArgs
	JSON bool   `arg:"--json" help:"print deterministic JSON"`
	Kind string `arg:"--kind" placeholder:"KIND" help:"only default, branch, or tag refs"`
	Repo string `arg:"positional" placeholder:"OWNER/REPO" help:"only refs for this source repository"`
}

func (refsArgs) Description() string {
	return "List the branches, tags, and resolved commits currently indexed by the remote service."
}

func runRefs(args []string) error {
	values := refsArgs{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple refs"}, &values)
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
	values.Kind = strings.ToLower(strings.TrimSpace(values.Kind))
	if values.Kind != "" && values.Kind != "default" && values.Kind != "branch" && values.Kind != "tag" {
		return fmt.Errorf("--kind must be default, branch, or tag")
	}
	entries, err := fetchRepoList(serverDefault(values.Server))
	if err != nil {
		return err
	}
	entries = refsForRepository(entries, values.Repo, values.Kind)
	if values.JSON {
		return stdoutWriter().writeJSON(api.ReposResponse{OK: true, Count: len(entries), Repos: entries})
	}
	return renderRefs(entries)
}

func renderRefs(entries []api.RepoListEntry) error {
	for _, entry := range entries {
		head := ""
		if entry.Head != nil {
			head = *entry.Head
		}
		selector := entry.Selector
		if selector == "" {
			selector = entry.Repo
		}
		if err := stdoutWriter().writeString(fmt.Sprintf("%s\t%s\t%s\t%s\n", selector, entry.RefKind, entry.Ref, head)); err != nil {
			return err
		}
	}
	if len(entries) == 0 {
		setExit(1)
	}
	return nil
}

func refsForRepository(entries []api.RepoListEntry, repo, kind string) []api.RepoListEntry {
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
