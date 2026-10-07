// Package hookruntime bridges separately built Pi integrations to the
// repository-owned, read-only GritQL hook runner.
package hookruntime

import (
	"context"

	"github.com/greppleai/grepple/internal/cli/hook"
)

// Finding is one normalized source-linked hook diagnostic.
type Finding = hook.Finding

// CheckRelation runs one configured relational hook against a complete local
// repository snapshot, including unchanged files needed for cross-file joins.
func CheckRelation(ctx context.Context, root, id string) ([]Finding, error) {
	return hook.CheckRepositoryRelation(ctx, root, id)
}

// CheckFileRule runs one configured file-local GritQL hook over the full scope.
func CheckFileRule(ctx context.Context, root, id string) ([]Finding, error) {
	return hook.CheckRepositoryFileRule(ctx, root, id)
}

// RepositoryRoot resolves the hook configuration ancestor for relative findings.
func RepositoryRoot(start string) (string, error) { return hook.RepositoryRoot(start) }
