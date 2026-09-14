---
name: local-grep-and-search
description: Use for locating local files, text, declarations, exact ranges, or edit-ready evidence in the current checkout. Replaces grep/rg/find-for-content. Do not use it alone for change impact, architecture ownership, boundary design, or syntax-shaped audits; use the dedicated Grepple skills for those decisions.
---

# Local evidence retrieval with Grepple

Use the smallest result shape that can answer the question. Grepple is deterministic path order, not relevance-ranked, so broad first-page results are not proof that later files are irrelevant.

## Pick one intent

| Need | Command |
| --- | --- |
| Measure breadth without bodies | `grepple -F 'Name' --count-summary SCOPE` |
| List matching files | `grepple -F 'Name' --files-with-matches SCOPE` |
| List files by path/glob | `grepple --files 'src/**/*.go'` |
| Explain selected/excluded paths | `grepple sources explain --compact SCOPE` |
| Inventory one file | `grepple --outline path/to/file.go` |
| Retrieve enclosing implementation | `grepple -F 'Name' SCOPE --limit 5` |
| Locate exact evidence lines | `grepple --line-only -F 'Name' SCOPE` |
| Find the owner of a body match | `grepple --line-only --enclosing -F 'call()' file.go` |
| Retrieve a known construct | `grepple --at path/to/file.go:START-END` |
| Emit edit anchors | add `--anchors`; use `--no-anchors` for plain output |

Default search returns enclosing structural segments and collapses unrelated bodies. Use `--line-only` only when the lines themselves are sufficient or when obtaining a range for the next `--at`/Read call.

## Evidence rules that prevent bad decisions

1. Start with `--count-summary` when scope is unknown. A first page is a sample, not repository-wide evidence. Default path order is cheapest; use `--sort matches` only for broad exploration because it scans the full selected universe.
2. Use `-F` for identifiers and snippets. The default pattern mode is JavaScript regex, not POSIX or RE2.
3. Distinguish `--files` (path discovery) from `--files-with-matches` (content evidence). `-l` means file listing here, not grep's matching-file behavior.
4. Treat a text occurrence as an occurrence only. For callers, dependencies, or refactor impact, switch to `change-impact-analysis`.
5. Treat `recovered`, `unsupported`, or `failed` source analysis as incomplete parser evidence; plain text is intentional but has no syntax guarantees.
6. In complete JSON, inspect `metadata.page.complete`, limits, omissions, and diagnostics; use its copyable `nextCommand` rather than inventing a paging command. If stdout is `grepple-artifact-v1`, inspect the descriptor first and read only relevant artifact ranges; use its `--no-spill` rerun only when the complete stream is required. Human truncation and omitted-segment messages are likewise incomplete.
7. Selected paths/globs and repository source controls define the evidence universe. If exclusions could matter, run `grepple sources explain --compact SCOPE`; use `--production-only` only for explicitly production-scoped questions. State that universe when making a completeness claim.
8. Do not read an entire large file after Grepple supplied an exact construct range.

## Edit-safe workflow

```bash
grepple --line-only --enclosing -F 'targetCall(' path/to/package
grepple --at path/to/file.go:40-65
```

With a configured anchor provider, structural and line output may already contain `HASH│LINE│content`; otherwise request `--anchors`. Hashes become stale after edits, so refresh before a later edit. Plain locations remain useful for Read and `--at`.

For local call impact use `change-impact-analysis`; for package ownership use `architecture-lookup-discovery`; for repeated architectural spread use `architecture-boundary-review`; for syntax-shaped matching use `structural-pattern-audit`. For code outside this checkout use `remote-grep-and-search` rather than cloning merely to inspect it.
