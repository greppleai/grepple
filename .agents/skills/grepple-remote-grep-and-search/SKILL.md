---
name: grepple-remote-grep-and-search
description: Use for indexed repositories, remote files/ranges, trees, or recurring questions across approved repositories outside the current checkout. Query explicit repositories; do not clone just to inspect code.
---

# Remote Grepple

## Resolve identity, then retrieve

1. If the repository identity is unknown, run `grepple repos <substring>` and choose an exact `OWNER/REPO` (and indexed ref when the question is version-specific). Stop if unavailable; do not widen to a corpus query.
2. Choose the cheapest retrieval:
   - Breadth: `grepple -R --repo OWNER/REPO -F 'Symbol' --count-summary`
   - Matching paths: `grepple -R --repo OWNER/REPO -F 'Symbol' --files-with-matches`
   - Known range: `grepple get OWNER/REPO path/to/file.go --lines 40:80`
   - Directory shape: `grepple tree OWNER/REPO [PATH] --depth N`
   - File map: `grepple get OWNER/REPO PATH --outline`
3. For a source-linked relation, use an exact repository selector and location: `grepple --server URL --repo OWNER/REPO --related --at path/file.go:40`. Treat candidate edges as leads and retain the repository/ref selector in the answer.

Search is local unless `-R` or `--server` enables remote search; `get`, `tree`, `repos`, and `rules` are remote commands. Remote pages cap at 100: use `--skip` or narrow the query, not a first page as complete evidence. For recurring searches across an approved set, use scoped saved rules. Use `grepple-local-grep-and-search` for this checkout and its evidence/completeness rules.
