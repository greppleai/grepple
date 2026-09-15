# Versioned remote indexing

An indexed repository can declare additional open-source repositories and refs in its root `grepple.json`:

```json
{
  "index": {
    "repositories": [
      {
        "repo": "sourcegraph/zoekt",
        "branches": ["main", "release/*"],
        "tags": ["v0.25.*"]
      }
    ]
  }
}
```

The backend trusts this repository-owned declaration during reconciliation. Declarations from all indexed checkouts are merged deterministically and cycles are harmless because each repository/ref selector is deduplicated. Newly discovered repositories can contribute their own configuration on a later reconciliation cycle.

The target repository's remote default branch is always indexed, even when no branch pattern matches it. Branch and tag patterns use Go path-style glob syntax. Each pattern set is bounded to 20 matching refs. Tags are preferred newest-first: valid semantic versions sort by semantic version, followed by other tag names in descending lexical order. Ref discovery reads at most 500 branches or tags from GitHub per target.

Use `grepple refs` to inspect the exact source ref and resolved commit:

```bash
grepple refs sourcegraph/zoekt
grepple refs --kind tag sourcegraph/zoekt
grepple refs --json sourcegraph/zoekt
```

Additional checkouts have stable selectors such as `sourcegraph/zoekt@tag~v0.25.0`. Pass the selector anywhere an indexed repository name is accepted:

```bash
grepple -R --repo 'sourcegraph/zoekt@tag~v0.25.0' -F 'NewDirectorySearcher'
grepple get 'sourcegraph/zoekt@tag~v0.25.0' search/shards.go --lines 1:120
grepple tree 'sourcegraph/zoekt@tag~v0.25.0' search --depth 2
```

Selectors escape slashes and other unsafe ref characters. The `refs` command is authoritative; callers should not construct selectors themselves. Its `head` field is the exact commit searched, which lets agents verify third-party source against a dependency version rather than the repository's current default branch.
