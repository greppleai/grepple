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
| Explain selected/excluded paths | `grepple sources explain SCOPE` |
| Inventory one file | `grepple --outline path/to/file.go` |
| Retrieve enclosing implementation | `grepple -F 'Name' SCOPE --limit 5` |
| Locate exact evidence lines | `grepple --line-only -F 'Name' SCOPE` |
| Find the owner of a body match | `grepple --line-only --enclosing -F 'call()' file.go` |
| Retrieve a known exact range | `grepple --line-only --at path/to/file.go:START-END` |
| Edit anchors | eligible local source output always uses native anchors; named compatibility providers come only from user settings |

Default search returns every complete enclosing function or method containing a direct match, with required owner wrappers but without proximity-selected imports, neighboring declarations, or nonmatching siblings. Use `--line-only` only when evidence lines suffice or to obtain an exact range for a focused `--at` lookup and subsequent `grepple write`.

## Evidence rules that prevent bad decisions

1. Start with `--count-summary` when scope is unknown. A first page is a sample, not repository-wide evidence. Default path order is cheapest; use `--sort matches` only for broad exploration because it scans the full selected universe.
2. Use `-F` for identifiers and snippets. The default pattern mode is JavaScript regex, not POSIX or RE2.
3. Distinguish `--files` (path discovery) from `--files-with-matches` (content evidence). `-l` means file listing here, not grep's matching-file behavior.
4. Treat a text occurrence as an occurrence only. For callers, dependencies, or refactor impact, switch to `change-impact-analysis`.
5. Treat `recovered`, `unsupported`, or `failed` source analysis as incomplete parser evidence; plain text is intentional but has no syntax guarantees.
6. In complete JSON, inspect `metadata.page.complete`, limits, omissions, and diagnostics; use its copyable `nextCommand` rather than inventing a paging command. If stdout is `grepple-artifact-v1`, inspect the descriptor first and read only relevant artifact ranges; use its `--no-spill` rerun only when the complete stream is required. Human output truncation remains incomplete, while structural search itself does not cap matching scopes.
7. Selected paths/globs and repository source controls define the evidence universe. If exclusions could matter, run `grepple sources explain SCOPE`; use `--production-only` only for explicitly production-scoped questions. State that universe when making a completeness claim.
8. Do not read an entire large file after Grepple supplied an exact construct range.

## Edit-safe workflow

```bash
grepple --line-only --enclosing -F 'targetCall(' path/to/package
grepple --at path/to/file.go:40-65
```

Eligible local structural, contextual, and line-only output always uses native `HASH│LINE│content` anchors. There is no per-search plain-output override. Hashes become stale after edits, so use fresh anchors returned by `grepple write` or retrieve a new focused range after other file changes. Line locations remain useful for `--at`; generic Read/Edit tools are fallbacks, not the default write workflow.

### Preferred write transport

After retrieving fresh anchors, prefer the literal heredoc transaction for multi-edit or multi-file work. It avoids JSON and shell escaping while compiling to the same strict, prevalidated `grepple-write-v1` transaction:

```sh
grepple write --root . <<'GREPPLE_WRITE_ab12'
::grepple file internal/foo.go
::grepple replace START END
func updated() {
    return "quotes ' ` $ stay literal"
}
::grepple end

::grepple file internal/bar.go
::grepple replace START END
replacement body
::grepple end
GREPPLE_WRITE_ab12
```

Use `::grepple replace START` for a one-line range and an empty body to delete that range. Ranges are inclusive: when replacing an `if` and its body, include its closing brace in the anchored range to avoid leaving a duplicate brace. The same envelope supports `::grepple create` and digest-guarded `::grepple delete SHA256`. If source contains an exact `::grepple end` line, add `--end-marker TOKEN` to `replace` or `create` and terminate with `::grepple end TOKEN`. Use strict JSON when a program is already generating `grepple-write-v1`; use `grepple write edit` for a single literal replacement. The abbreviated `grepple write --help` synopsis may list only `edit`; stdin transactions still support `replace`, `create`, and `delete` (verify with `--dry-run`). All modes share confinement, original-snapshot anchors, transaction-wide validation, dry-run, structured output, rollback, and fresh post-write anchors.

For local call impact use `change-impact-analysis`; for package ownership use `architecture-lookup-discovery`; for repeated architectural spread use `architecture-boundary-review`; for syntax-shaped matching use `structural-pattern-audit`. For code outside this checkout use `remote-grep-and-search` rather than cloning merely to inspect it.
