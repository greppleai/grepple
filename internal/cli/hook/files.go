package hook

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/pathfilter"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func selectedFiles(ctx context.Context, root string, all bool, policy sourcedomain.Options) ([]string, error) {
	if all {
		listed, err := sourcedomain.Listing(ctx, []string{root}, sourcedomain.DiscoveryOptions{
			Root: root, IgnoreRoot: policy.Root(), IgnorePaths: policy.IgnorePaths, ProductionOnly: policy.ProductionOnly,
		})
		if err != nil {
			return nil, err
		}
		return regularFiles(root, listed, policy), nil
	}
	gitRoot, err := gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("changed-file hooks require a Git repository (use --all for a full scan): %w", err)
	}
	if filepath.Clean(strings.TrimSpace(string(gitRoot))) != root {
		return nil, fmt.Errorf("hook root %s differs from Git root %s", root, strings.TrimSpace(string(gitRoot)))
	}
	unique := map[string]bool{}
	// One porcelain status includes staged, unstaged, and untracked paths while
	// verifying stale index entries. Git's index and untracked caches reduce the
	// cost of repeated checks without changing the selected file set.
	changed, err := gitOutput(ctx, root, "-c", "core.preloadIndex=true", "-c", "core.untrackedCache=true", "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("discover changed files: %w", err)
	}
	for _, entry := range bytes.Split(changed, []byte{0}) {
		if len(entry) >= 4 && entry[2] == ' ' {
			unique[string(entry[3:])] = true
		}
	}
	listed := make([]string, 0, len(unique))
	for path := range unique {
		listed = append(listed, path)
	}
	return regularFiles(root, listed, policy), nil
}

func gitOutput(ctx context.Context, root string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func regularFiles(root string, paths []string, policy sourcedomain.Options) []string {
	ignored := pathfilter.Config{Root: policy.Root(), Patterns: policy.IgnorePaths}
	classifier := sourcedomain.NewClassifier(policy.Root())
	selected := make([]string, 0, len(paths))
	seen := make(map[string]bool)
	for _, path := range paths {
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(root, filepath.FromSlash(path))
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == "." {
			continue
		}
		info, err := os.Lstat(absolute)
		if err != nil || !info.Mode().IsRegular() || ignored.Ignored(absolute) || policy.ProductionOnly && classifier.Classify(absolute) != sourcedomain.Production {
			continue
		}
		path = filepath.ToSlash(relative)
		if !seen[path] {
			seen[path] = true
			selected = append(selected, path)
		}
	}
	sort.Strings(selected)
	return selected
}
