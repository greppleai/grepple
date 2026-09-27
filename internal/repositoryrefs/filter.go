// Package repositoryrefs contains command-neutral indexed-reference projections.
package repositoryrefs

import (
	"strings"

	"github.com/greppleai/grepple/internal/wire"
)

// Filter selects references by source repository and reference kind.
func Filter(entries []wire.RepoListEntry, repo, kind string) []wire.RepoListEntry {
	repo = strings.TrimSpace(repo)
	kind = strings.TrimSpace(strings.ToLower(kind))
	if repo == "" && kind == "" {
		return entries
	}
	kept := make([]wire.RepoListEntry, 0, len(entries))
	for _, entry := range entries {
		if (repo == "" || entry.Repo == repo) && (kind == "" || entry.RefKind == kind) {
			kept = append(kept, entry)
		}
	}
	return kept
}
