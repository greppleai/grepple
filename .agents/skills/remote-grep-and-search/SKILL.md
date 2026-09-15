---
name: remote-grep-and-search
description: "Use when the target is outside the current checkout: an indexed repository, remote file/range, repository tree, or recurring question across an approved repository set. Query explicit repositories; do not clone merely to inspect code or fall back to grep/rg/find. Use local-grep-and-search for the current working directory."
---

# Remote Grepple

Remote Grepple exists to inspect indexed code without cloning. Treat repository identity as a required boundary: broad corpus queries are noisy, expensive, and can cross authorization or product expectations.

## Decide by intent

- **Repo identity unknown:** use `grepple repos <substring>` first.
- **Need to locate a term:** query an explicit `OWNER/REPO` with `-R --repo`, preferably `--count-summary` for complete breadth or `--files-with-matches` for paths before bodies.
- **Need one known file/range:** use `grepple get OWNER/REPO PATH --lines A:B`; this is cheaper and more deterministic than another search.
- **Need repository shape:** use `grepple tree OWNER/REPO [PATH] --depth N`.
- **Need a structural map:** fetch with `grepple get OWNER/REPO PATH --outline`.
- **Need a recurring answer across an approved repository set:** use scoped saved `rules` searches so indexing work is materialized rather than repeated.

## Important boundary

Always scope remote content search to an exact repository:

```bash
grepple -R --repo OWNER/REPO -F 'Symbol' --count-summary
grepple -R --repo OWNER/REPO -F 'Symbol' --files-with-matches
grepple -R --repo OWNER/REPO -F 'Symbol' --limit 5
grepple get OWNER/REPO path/to/file.go --lines 40:80
```

Do not issue unrestricted corpus-wide searches. If the repository is unavailable or ambiguous, stop and resolve it with `repos` rather than widening the query.

## Remote navigation

`--related` and `--follow-related` build navigation from the complete selected indexed checkout rather than only text-match candidates. Always provide an exact `--repo` selector. Remote `--at` also requires exactly one repository and accepts a repository-relative location:

```bash
grepple --server URL --repo OWNER/REPO --related -F 'Symbol'
grepple --server URL --repo OWNER/REPO --at path/file.go:40
```

Navigation remains syntax-based and bounded. Treat candidates as leads, preserve the exact repository/ref selector, and use `get --lines` when only one known range is needed.

## Operational facts

- Search is local unless `-R/--remote` or `--server` is supplied; `get`, `tree`, `repos`, and `rules` are inherently remote.
- Remote pages are capped at 100; page deterministically with `--skip` rather than requesting an unbounded dump.
- When run inside a checkout, combined local+remote search excludes that checkout's remote repository to prevent duplicates. To query its indexed copy, run elsewhere or use `get`/`tree`.
- A configured server does not itself enable remote search.
- Truncation means the sample is incomplete; narrow the repository, path, or pattern before drawing conclusions.

Use the local skill for detailed search semantics, structural output, globs, and navigation confidence interpretation.
