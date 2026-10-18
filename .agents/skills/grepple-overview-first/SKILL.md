---
name: grepple-overview-first
description: Use at the start of a new session or when entering an unfamiliar local checkout to inspect current Grepple annotation coverage and vocabulary with grepple overview before scoped tree/search. Never run metadata generation automatically.
---

# Start local repository discovery with overview

1. From the repository root run `grepple overview` **first**. It reads local source selection and checksums, not a model. Read the selected/current/stale/missing/invalid file counts and the actual purpose, domain-area, functional-area and keyword labels. Counts are complete; shown labels and example paths are bounded. If this CLI lacks `overview`, continue with `grepple tree --depth 1` and unfiltered focused search.
2. If annotations are current, select a *real* label from the output, then narrow a promising subtree:
   ```sh
   grepple tree --purpose implementation --depth 2 PATH
   grepple --purpose implementation --domain-area source-parsing --files PATH
   grepple --functional-area language-adapters --keyword go --line-only -F 'Parse' PATH
   ```
   Replace example labels and PATH with observed values. Repeat a flag to OR values within its category; distinct categories AND **on the same selected, checksum-current file**. Labels are lowercase with hyphens on disk, but matching also accepts spaces/case variants. Use `--keyword` for exact labels, not semantic/fuzzy search. `--outline --kind functions` is a separate syntax-shape selector, not a file-purpose filter.
3. If overview shows missing/stale/invalid coverage, do not infer absence from a filtered no-match. Use unfiltered `grepple tree --depth 1`, `grepple verify`, `grepple sources explain PATH`, and focused search/outline/at for the gap. `grepple init --only-directory PATH` invokes a configured model and is **opt-in only**; never trigger it just to orient, and never run a full-checkout init without approval.
4. Metadata selectors work on **local tree and search only**. Remote indexes, piped stdin, cursor/repo/server searches, and explicit related navigation cannot combine with them. `graph`, `grit`, `architecture`, `ask`, `get`, `refs`, and `rules` have separate parsers and are not narrowed by these flags. Confirm source and call edges with exact `--at` and focused graph commands; labels are navigation leads, not proof of ownership or exhaustive results.

For retrieval details load `grepple-local-grep-and-search`; for package ownership load `grepple-architecture-lookup-discovery`.