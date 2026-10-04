// Package hook runs repository-owned, read-only GritQL checks.
package hook

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/greppleai/grepple/internal/config"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

// CheckRepositoryFileRule evaluates one file-local rule over all selected sources,
// with the same cache, annotation and suppression semantics as `grepple hook`.
func CheckRepositoryFileRule(ctx context.Context, start, id string) ([]Finding, error) {
	root, err := hookRoot(start)
	if err != nil {
		return nil, err
	}
	rules, err := loadRules(root, []string{id})
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) != 1 || rules[0].program == nil {
		return nil, fmt.Errorf("hook %s is not file-local", id)
	}
	policy, err := repositorySourcePolicy(root)
	if err != nil {
		return nil, err
	}
	paths, err := selectedFiles(ctx, root, true, policy)
	if err != nil {
		return nil, err
	}
	found, err := scanRules(ctx, root, paths, rules, true, maxHookWorkers)
	if err != nil {
		return nil, err
	}
	return filterSuppressedFindings(root, found, rules...)
}
func repositorySourcePolicy(root string) (sourcedomain.Options, error) {
	settings, err := config.LoadConfig(root, false)
	if err != nil {
		return sourcedomain.Options{}, err
	}
	policy := sourcedomain.Options{WorkingDirectory: root, IgnoreRoot: root}
	if settings.RepositoryPath != "" {
		policy.IgnoreRoot = filepath.Dir(settings.RepositoryPath)
		policy.IgnorePaths = settings.Repository.Ignore.Paths
	}
	return policy, nil
}
