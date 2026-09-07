package repository

import (
	"grepple/internal/api"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Registry is a shard's in-memory repository table rooted at one directory:
// which repos exist, where their checkouts live, and their freshness metadata.
// It is safe for concurrent use.
type Registry struct {
	root  string
	mu    sync.RWMutex
	repos map[string]api.RepoInfo
}

// NewRegistry returns an empty Registry rooted at root. Call Init to adopt
// clones that already exist on disk.
func NewRegistry(root string) *Registry {
	return &Registry{root: root, repos: map[string]api.RepoInfo{}}
}

// Dir returns the filesystem path of a repository's checkout
// (root/owner/name); the path is returned whether or not it exists.
func (r *Registry) Dir(repo string) string {
	return filepath.Join(r.root, filepath.FromSlash(repo))
}

// Init creates the root directory and adopts every on-disk clone found under
// it (owner/name directories containing .git), recording each clone's URL,
// current branch, and head commit from git.
func (r *Registry) Init() error {
	if e := os.MkdirAll(r.root, 0755); e != nil {
		return e
	}
	owners, _ := os.ReadDir(r.root)
	for _, o := range owners {
		if !o.IsDir() {
			continue
		}
		names, _ := os.ReadDir(filepath.Join(r.root, o.Name()))
		for _, n := range names {
			dir := filepath.Join(r.root, o.Name(), n.Name())
			if _, e := os.Stat(filepath.Join(dir, ".git")); e != nil {
				continue
			}
			id := o.Name() + "/" + n.Name()
			url, ref, head := "", "HEAD", gitValue(dir, "rev-parse", "HEAD")
			if x := gitValue(dir, "remote", "get-url", "origin"); x != nil {
				url = *x
			}
			if x := gitValue(dir, "rev-parse", "--abbrev-ref", "HEAD"); x != nil {
				ref = *x
			}
			r.repos[id] = api.RepoInfo{
				Repo:      id,
				URL:       url,
				Ref:       ref,
				Dir:       dir,
				Head:      head,
				IndexedAt: isoNow(),
			}
		}
	}
	return nil
}

// List returns every registered repository sorted by name (deterministic,
// diff-friendly order; no ranking).
func (r *Registry) List() []api.RepoInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]api.RepoInfo, 0, len(r.repos))
	for _, x := range r.repos {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Repo < out[j].Repo
	})
	return out
}

// Get returns the api.RepoInfo for repo; ok is false when the repository is
// unknown or the name fails sanitization.
func (r *Registry) Get(repo string) (api.RepoInfo, bool) {
	id, e := sanitizeRepoID(repo)
	if e != nil {
		return api.RepoInfo{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.repos[id]
	return x, ok
}

// Record inserts or replaces a repository's entry, stamping it with the
// checkout's current head commit and the time. Call it after any operation
// that moves the checkout (clone, pull).
func (r *Registry) Record(id, url, ref, dir string) api.RepoInfo {
	x := api.RepoInfo{
		Repo:      id,
		URL:       url,
		Ref:       ref,
		Dir:       dir,
		Head:      gitValue(dir, "rev-parse", "HEAD"),
		IndexedAt: isoNow(),
	}
	r.mu.Lock()
	r.repos[id] = x
	r.mu.Unlock()
	return x
}

// Add shallow-clones a repository into the registry root and records it. The
// remote's default branch always wins over a caller-supplied ref (which can be
// stale). A repository whose checkout already exists on disk is returned
// unchanged.
func (r *Registry) Add(repo, url, ref, token string) (api.RepoInfo, error) {
	id, e := sanitizeRepoID(repo)
	if e != nil {
		return api.RepoInfo{}, e
	}
	if x, ok := r.Get(id); ok {
		if _, e = os.Stat(x.Dir); e == nil {
			return x, nil
		}
	}
	if url == "" {
		url = DefaultGithubURL(id)
	}
	dir := r.Dir(id)
	os.MkdirAll(filepath.Dir(dir), 0755)
	os.RemoveAll(dir)
	// Clone without --branch so git checks out the remote's default branch (its
	// HEAD). The caller-supplied ref (e.g. a discovered default_branch) can be
	// stale or wrong for repos that do not use "main", and passing it to
	// --branch would fail the clone; letting the remote decide is authoritative.
	args := []string{"clone", "--depth", "1", url, dir}
	if _, e = git(args, "", token); e != nil {
		return api.RepoInfo{}, e
	}
	// Record the branch actually checked out from the remote default.
	if x := gitValue(dir, "rev-parse", "--abbrev-ref", "HEAD"); x != nil && *x != "" {
		ref = *x
	} else if ref == "" {
		ref = "HEAD"
	}
	return r.Record(id, url, ref, dir), nil
}

// Pull fast-forwards a repository's checkout to the remote ref (shallow fetch
// + reset --hard to FETCH_HEAD) and re-records it. A repository with no
// checkout on disk is cloned via Add instead.
func (r *Registry) Pull(repo, url, ref, token string) (api.RepoInfo, error) {
	id, e := sanitizeRepoID(repo)
	if e != nil {
		return api.RepoInfo{}, e
	}
	dir := r.Dir(id)
	if _, e = os.Stat(filepath.Join(dir, ".git")); e != nil {
		return r.Add(id, url, ref, token)
	}
	old, _ := r.Get(id)
	if ref == "" {
		ref = old.Ref
		if ref == "" {
			if x := gitValue(dir, "rev-parse", "--abbrev-ref", "HEAD"); x != nil {
				ref = *x
			}
		}
	}
	if _, e = git([]string{"-C", dir, "fetch", "--depth", "1", "origin", ref}, dir, token); e != nil {
		return api.RepoInfo{}, e
	}
	if _, e = git([]string{"-C", dir, "reset", "--hard", "FETCH_HEAD"}, dir, ""); e != nil {
		return api.RepoInfo{}, e
	}
	if url == "" {
		url = old.URL
	}
	return r.Record(id, url, ref, dir), nil
}

// Remove deletes a repository's entry and its checkout from disk, reporting
// whether anything existed (registered or on disk).
func (r *Registry) Remove(repo string) (bool, error) {
	id, e := sanitizeRepoID(repo)
	if e != nil {
		return false, e
	}
	r.mu.Lock()
	_, ok := r.repos[id]
	delete(r.repos, id)
	r.mu.Unlock()
	_, se := os.Stat(r.Dir(id))
	existed := ok || se == nil
	return existed, os.RemoveAll(r.Dir(id))
}
