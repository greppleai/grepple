package repository

import (
	"fmt"
	"grepple/internal/api"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func sanitizeRepoID(repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	if !regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`).MatchString(repo) {
		return "", fmt.Errorf("invalid repo id (expected 'owner/name'): %s", repo)
	}
	return repo, nil
}

// DefaultGithubURL returns the HTTPS clone URL for an owner/name repository
// when no explicit clone URL is configured.
func DefaultGithubURL(repo string) string {
	return "https://github.com/" + repo + ".git"
}

// SafeRepoFilePath resolves a client-supplied path against a repository's
// checkout directory, rejecting absolute paths, escapes outside the checkout,
// the checkout root itself, and anything under .git. ok is false for every
// rejected form.
func SafeRepoFilePath(dir, requested string) (string, bool) {
	if filepath.IsAbs(requested) {
		return "", false
	}
	base, _ := filepath.Abs(dir)
	abs, _ := filepath.Abs(filepath.Join(base, requested))
	rel, e := filepath.Rel(base, abs)
	if e != nil || rel == "." || rel == "" || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) || strings.Split(rel, string(filepath.Separator))[0] == ".git" {
		return "", false
	}
	return abs, true
}

// WalkTree lists a directory's entries up to depth levels deep (depth 2 = the
// top two levels), sorted by name at each level and excluding .git — the
// shard's /tree endpoint payload.
func WalkTree(base string, depth int) []api.TreeEntry {
	var out []api.TreeEntry
	var walk func(string, string, int)
	walk = func(dir, prefix string, d int) {
		items, e := os.ReadDir(dir)
		if e != nil {
			return
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].Name() < items[j].Name()
		})
		for _, x := range items {
			if x.Name() == ".git" {
				continue
			}
			rel := x.Name()
			if prefix != "" {
				rel = prefix + "/" + x.Name()
			}
			out = append(out, api.TreeEntry{Path: rel, Dir: x.IsDir()})
			if x.IsDir() && d > 1 {
				walk(filepath.Join(dir, x.Name()), rel, d-1)
			}
		}
	}
	walk(base, "", depth)
	return out
}
