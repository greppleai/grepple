package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"grepple/internal/api"
	"grepple/internal/repository"
)

func rebalance(o routerOptions, dry bool) map[string]any {
	indexed := listIndexed(o)
	planned := []map[string]string{}
	moved := []map[string]string{}
	failed := []map[string]string{}
	for _, x := range indexed {
		to := o.ring.get(x.Repo)
		if to == x.Shard {
			continue
		}
		move := map[string]string{"repo": x.Repo, "from": x.Shard, "to": to}
		planned = append(planned, move)
		if dry {
			continue
		}
		status, _ := indexShard(o, to, http.MethodPut, api.RepoRef{
			Repo: x.Repo,
			URL:  x.URL,
			Ref:  x.Ref,
		})
		if status >= 400 {
			failed = append(failed, map[string]string{"repo": x.Repo, "error": fmt.Sprintf("copy to %s failed (%d)", to, status)})
			continue
		}
		status, _ = indexShard(o, x.Shard, "DELETE", api.RepoRef{Repo: x.Repo})
		if status >= 400 {
			failed = append(failed, map[string]string{"repo": x.Repo, "error": fmt.Sprintf("delete from %s failed (%d); duplicate remains", x.Shard, status)})
			continue
		}
		moved = append(moved, move)
	}
	return map[string]any{"planned": planned, "moved": moved, "failed": failed}
}

type githubRepo struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	Disabled      bool   `json:"disabled"`
	PushedAt      string `json:"pushed_at"`
}

// listOrgRepos lists every repository of a GitHub organization (or user),
// trying the /orgs form first and falling back to /users. A 404 on both forms
// yields an empty list (the org simply does not exist).
func listOrgRepos(org, token string) ([]githubRepo, error) {
	base := environment("GITHUB_API_URL", "https://api.github.com")
	paths := []string{"/orgs/" + url.PathEscape(org) + "/repos", "/users/" + url.PathEscape(org) + "/repos"}
	for pi, path := range paths {
		all, notFound, err := listReposPath(base, path, token)
		if err != nil {
			return nil, err
		}
		if !notFound || pi == len(paths)-1 {
			return all, nil
		}
	}
	return []githubRepo{}, nil
}

// listReposPath pages one GitHub repository-list endpoint (100 per page, at
// most 20 pages). notFound reports a 404 so the caller can try the other
// org/user form.
func listReposPath(base, path, token string) (all []githubRepo, notFound bool, err error) {
	for page := 1; page <= 20; page++ {
		target := fmt.Sprintf("%s%s?per_page=100&page=%d&type=all", strings.TrimRight(base, "/"), path, page)
		req, _ := http.NewRequest("GET", target, nil)
		req.Header.Set("accept", "application/vnd.github+json")
		req.Header.Set("user-agent", "grepple-router")
		req.Header.Set("x-github-api-version", "2022-11-28")
		if token != "" {
			req.Header.Set("authorization", "Bearer "+token)
		}
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			return nil, false, e
		}
		if resp.StatusCode == 404 {
			resp.Body.Close()
			return nil, true, nil
		}
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, false, fmt.Errorf("GitHub API %d for %s: %s", resp.StatusCode, path, string(b[:min(len(b), 200)]))
		}
		var batch []githubRepo
		e = json.NewDecoder(resp.Body).Decode(&batch)
		resp.Body.Close()
		if e != nil {
			return nil, false, e
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return all, false, nil
}

// listInstallationRepos lists the repositories accessible to a GitHub App
// installation using its installation access token.
func listInstallationRepos(apiBase, token string) ([]githubRepo, error) {
	base := strings.TrimRight(environmentOr(apiBase, "https://api.github.com"), "/")
	var all []githubRepo
	for page := 1; page <= 20; page++ {
		target := fmt.Sprintf("%s/installation/repositories?per_page=100&page=%d", base, page)
		req, _ := http.NewRequest("GET", target, nil)
		req.Header.Set("accept", "application/vnd.github+json")
		req.Header.Set("user-agent", "grepple-router")
		req.Header.Set("x-github-api-version", "2022-11-28")
		req.Header.Set("authorization", "Bearer "+token)
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			return nil, e
		}
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API %d for /installation/repositories: %s", resp.StatusCode, string(b[:min(len(b), 200)]))
		}
		var batch struct {
			Repositories []githubRepo `json:"repositories"`
		}
		e = json.NewDecoder(resp.Body).Decode(&batch)
		resp.Body.Close()
		if e != nil {
			return nil, e
		}
		all = append(all, batch.Repositories...)
		if len(batch.Repositories) < 100 {
			break
		}
	}
	return all, nil
}

// environmentOr returns value when non-empty, otherwise fallback.
func environmentOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// discoveryStats aggregates a cycle's admission counters so the gap between
// an org's total repositories and what we index is visible without logging
// every repository (see docs/logging.md).
type discoveryStats struct {
	discovered, archived, disabled, otherOrg, filtered int
}

// repoAllowlist seeds desired with the WATCH_REPOS entries and returns the
// admission predicate. When watchRepos is non-empty it is a HARD allowlist:
// discovery (watchOrgs + GitHub App installations) may only add repositories
// that are in it — identically for PAT and App auth. Discovery is still used
// to resolve each allowed repo's clone URL and default branch. An empty
// allowlist admits everything discovery returns.
func repoAllowlist(watchRepos []string, desired map[string]api.RepoRef) func(string) bool {
	allow := map[string]bool{}
	for _, repo := range watchRepos {
		allow[repo] = true
		desired[repo] = api.RepoRef{Repo: repo, URL: repository.DefaultGithubURL(repo)}
	}
	return func(fullName string) bool { return len(allow) == 0 || allow[fullName] }
}

// admitRepo filters one discovered repository (archived, disabled, outside the
// installation's org scope, outside the allowlist) and records it into
// desired/pushedAt when admitted.
func admitRepo(r githubRepo, orgScope string, allowed func(string) bool, desired map[string]api.RepoRef, pushedAt map[string]time.Time, stats *discoveryStats) {
	switch {
	case r.Archived:
		stats.archived++
	case r.Disabled:
		stats.disabled++
	case orgScope != "" && !strings.EqualFold(orgOf(r.FullName), orgScope):
		stats.otherOrg++
	case !allowed(r.FullName):
		stats.filtered++
	default:
		desired[r.FullName] = api.RepoRef{Repo: r.FullName, URL: r.CloneURL, Ref: r.DefaultBranch}
		if ts, err := time.Parse(time.RFC3339, r.PushedAt); err == nil {
			pushedAt[r.FullName] = ts
		}
	}
}

// discoverFromOrgs admits every repository of each watched org; a failed org
// listing is reported and skipped so other sources still contribute.
func discoverFromOrgs(orgs []string, token string, allowed func(string) bool, desired map[string]api.RepoRef, pushedAt map[string]time.Time, stats *discoveryStats) []string {
	var errs []string
	for _, org := range orgs {
		repos, e := listOrgRepos(org, token)
		if e != nil {
			errs = append(errs, org+": "+e.Error())
			continue
		}
		stats.discovered += len(repos)
		for _, r := range repos {
			admitRepo(r, "", allowed, desired, pushedAt, stats)
		}
	}
	return errs
}

// discoverFromInstallations mints each GitHub App installation's token and
// admits the repositories it can access, scoped to the installation's
// configured org.
func discoverFromInstallations(installations []*installationToken, allowed func(string) bool, desired map[string]api.RepoRef, pushedAt map[string]time.Time, stats *discoveryStats) []string {
	var errs []string
	for _, inst := range installations {
		token, e := inst.get()
		if e != nil {
			errs = append(errs, fmt.Sprintf("app %d installation %d: %v", inst.appID, inst.installationID, e))
			continue
		}
		repos, e := listInstallationRepos(inst.apiBase, token)
		if e != nil {
			errs = append(errs, fmt.Sprintf("%s (installation %d): %v", inst.org, inst.installationID, e))
			continue
		}
		stats.discovered += len(repos)
		for _, r := range repos {
			admitRepo(r, inst.org, allowed, desired, pushedAt, stats)
		}
	}
	return errs
}

// pruneUndesired deletes indexed repositories that discovery no longer wants
// (archived, disabled, deleted, transferred, or made inaccessible). safe is
// false when a discovery source errored, and pruning is skipped when nothing
// is desired, so a transient GitHub API failure can never wipe the index.
// Deletion targets the shard that actually holds the repo (x.Shard), mirroring
// rebalance.
func pruneUndesired(o routerOptions, idx []indexedRepo, desired map[string]api.RepoRef, safe bool) (pruned, errs []string) {
	if !safe || len(desired) == 0 {
		return nil, nil
	}
	for _, x := range idx {
		if _, want := desired[x.Repo]; want {
			continue
		}
		status, _ := indexShard(o, x.Shard, http.MethodDelete, api.RepoRef{Repo: x.Repo})
		if status >= 400 {
			errs = append(errs, fmt.Sprintf("prune %s from %s failed (%d)", x.Repo, x.Shard, status))
			continue
		}
		pruned = append(pruned, x.Repo)
	}
	sort.Strings(pruned)
	return pruned, errs
}

// selectReposToEnqueue returns, in deterministic order, the repositories that
// should be enqueued this cycle: those that are desired but neither already
// indexed (present) nor already in flight (inflight). When batch > 0 at most
// batch repositories are returned and the remainder are reported as deferred to
// later cycles, pacing the initial sync of very large orgs.
func selectReposToEnqueue(desired map[string]api.RepoRef, present, inflight map[string]bool, batch int) (queue []string, deferred int) {
	names := make([]string, 0, len(desired))
	for repo := range desired {
		names = append(names, repo)
	}
	sort.Strings(names)
	for _, repo := range names {
		if present[repo] || inflight[repo] {
			continue
		}
		if batch > 0 && len(queue) >= batch {
			deferred++
			continue
		}
		queue = append(queue, repo)
	}
	return queue, deferred
}

// selectStaleReposToRefresh returns, in deterministic order, the indexed
// repositories whose upstream pushed_at is newer than their shard's indexedAt
// — i.e. repos whose webhook-driven refresh was missed. Repos without
// pushed_at data (allowlist-only entries skip discovery) or with a job in
// flight are skipped; a missing/unparseable indexedAt counts as stale. batch
// caps the queue (0 = unlimited); it shares the cycle's budget with new repos.
func selectStaleReposToRefresh(pushedAt map[string]time.Time, indexed []indexedRepo, inflight map[string]bool, batch int) (queue []string, deferred int) {
	stale := []string{}
	for _, x := range indexed {
		pushed, ok := pushedAt[x.Repo]
		if !ok || inflight[x.Repo] {
			continue
		}
		indexedAt, err := time.Parse(time.RFC3339, x.IndexedAt)
		if err != nil || pushed.After(indexedAt) {
			stale = append(stale, x.Repo)
		}
	}
	sort.Strings(stale)
	for _, repo := range stale {
		if batch > 0 && len(queue) >= batch {
			deferred++
			continue
		}
		queue = append(queue, repo)
	}
	return queue, deferred
}
