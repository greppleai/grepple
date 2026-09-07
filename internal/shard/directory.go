package shard

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// directoryFile is the on-disk format of directory.json: the repositories
// currently indexed on this shard and the commit (head sha) each was indexed at.
type directoryFile struct {
	Repos []directoryRepo `json:"repos"`
}

type directoryRepo struct {
	Repo string `json:"repo"`
	Head string `json:"head"`
}

// directory tracks, and persists to directory.json, which repositories are
// indexed on this shard and at which commit. It lets the shard start fast:
// existing Zoekt shards are trusted immediately and only repositories whose head
// changed (or whose shard is missing) are re-indexed, asynchronously, in the
// background.
type directory struct {
	path    string
	mu      sync.Mutex
	entries map[string]string // repo -> head sha
}

// loadDirectory reads directory.json if present; a missing or corrupt file
// yields an empty directory (everything will be treated as needing indexing).
func loadDirectory(path string) *directory {
	d := &directory{path: path, entries: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return d
	}
	var file directoryFile
	if json.Unmarshal(data, &file) == nil {
		for _, r := range file.Repos {
			if r.Repo != "" {
				d.entries[r.Repo] = r.Head
			}
		}
	}
	return d
}

// head returns the indexed head sha recorded for repo, if any.
func (d *directory) head(repo string) (string, bool) {
	if d == nil {
		return "", false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.entries[repo]
	return h, ok
}

// set records repo at head and persists the directory.
func (d *directory) set(repo, head string) error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	if d.entries[repo] == head {
		d.mu.Unlock()
		return nil
	}
	d.entries[repo] = head
	d.mu.Unlock()
	return d.save()
}

// remove drops repo from the directory and persists it.
func (d *directory) remove(repo string) error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	_, existed := d.entries[repo]
	delete(d.entries, repo)
	d.mu.Unlock()
	if !existed {
		return nil
	}
	return d.save()
}

// save atomically writes directory.json (temp file + rename).
func (d *directory) save() error {
	d.mu.Lock()
	file := directoryFile{Repos: make([]directoryRepo, 0, len(d.entries))}
	for repo, head := range d.entries {
		file.Repos = append(file.Repos, directoryRepo{Repo: repo, Head: head})
	}
	d.mu.Unlock()
	sort.Slice(file.Repos, func(i, j int) bool { return file.Repos[i].Repo < file.Repos[j].Repo })
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, d.path)
}
