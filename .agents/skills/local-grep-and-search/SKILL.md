---
name: local-grep-and-search
description: Use for locating local files, text, declarations, exact ranges, or edit-ready evidence in the current checkout. Use specialized skills for impact, ownership, boundaries, and syntax-shaped audits.
---

# Local evidence retrieval with Grepple

## Choose the smallest result

| Question | Command |
| --- | --- |
| Measure breadth without bodies | `grepple -F 'Name' --count-summary SCOPE` |
| List matching files | `grepple -F 'Name' --files-with-matches SCOPE` |
| List paths by glob | `grepple --files 'src/**/*.go'` |
| Explain selected/excluded paths | `grepple sources explain SCOPE` |
| Inventory declarations | `grepple --outline path/to/file.go` |
| Retrieve enclosing implementation | `grepple -F 'Name' SCOPE --limit 5` |
| Locate exact evidence lines | `grepple --line-only -F 'Name' SCOPE` |
| Find the owner of a body match | `grepple --line-only --enclosing -F 'call()' file.go` |
| Retrieve a known range | `grepple --at path/to/file.go:START-END` |
| Edit anchors | Eligible local source output uses native anchors; named compatibility providers come only from user settings |

Use `-F` for literal names: default patterns are JavaScript regex. Default search returns enclosing functions/methods, not just matched lines; use `--line-only` when lines suffice. `--files` lists paths, while `--files-with-matches` lists content hits (`-l` means file listing). Results are in path order, not relevance order: an early page is not proof of completeness.

## Before a completeness claim

Start with `--count-summary` when scope is unknown. Check selected paths and exclusions with `grepple sources explain SCOPE` if they could matter; use `--production-only` only when tests and fixtures are intentionally excluded. In JSON check page completeness, omissions, and source diagnostics; use the supplied `nextCommand` to page. If output is a `grepple-artifact-v1` descriptor, inspect it before retrieving only relevant ranges. Recovered, failed, or unsupported parsing limits structural conclusions. A text occurrence is not a caller: use `change-impact-analysis` for that question.

## Edit from fresh anchors

```sh
grepple --line-only --enclosing -F 'targetCall(' path/to/package
grepple --at path/to/file.go:40-65
grepple write --root . <<'GREPPLE_WRITE_example'
::grepple file path/to/file.go
::grepple replace START END
replacement text
::grepple end
GREPPLE_WRITE_example
```

Use the `HASH│LINE│content` anchors returned by local Grepple; they become stale after an edit. Replace inclusive ranges (include a closing brace when replacing its block). The write receipt gives the changed range; retrieve fresh anchors before another edit or use `--return`. For create/delete, multi-file transactions, or marker escaping, follow the repository's Grepple write instructions and verify with `--dry-run`; the short `grepple write --help` synopsis does not document every transaction operation. Do not read an entire large file when an exact construct range is available.

Use `architecture-lookup-discovery` for ownership, `architecture-boundary-review` for proposed moves, `structural-pattern-audit` for syntax shapes, and `remote-grep-and-search` for indexed code outside this checkout.
