---
name: grepple-local-grep-and-search
description: Use Grepple for local file/content search (grep, rg, find, fd, ack; Claude Grep/Glob), including after the search guard blocks a tool. Locate declarations, exact ranges, and clean edit-ready source.
---

# Local evidence retrieval with Grepple

At the start of a new local session, run `grepple overview` to learn real labels and coverage without a model call (see `grepple-overview-first`). Then use `grepple tree --depth 1` and narrow a promising subtree; if metadata is missing/stale, use unfiltered tree/search instead of treating an empty filtered result as absence.

- **Literal text:** use `-F`; default patterns are JavaScript regex.
- **Matching lines:** use `--line-only`; default output shows enclosing bodies.
- **Verified local selectors:** when an outline or annotated search shows `@KIND:START-END`, read with `grepple --at PATH @KIND:START-END` to check current syntax and obtain clean source. `--at PATH:START-END` reads literal bounds without verifying a kind; graph and remote commands use their own selectors.

## Choose the smallest result
| Question | Command |
| --- | --- |
| Find current annotated implementation files | `grepple --purpose implementation --files PATH` (local, exact label; compare against unfiltered files when coverage is incomplete) |
| Narrow two categories on one current file | `grepple --domain-area source-parsing --functional-area language-adapters --files PATH` (replace with labels shown by `overview`) |
| Measure breadth without bodies | `grepple -F 'Name' --count-summary SCOPE` |
| List matching files | `grepple -F 'Name' --files-with-matches SCOPE` |
| List paths by glob | `grepple --files 'src/**/*.go'` |
| Explain selected/excluded paths | `grepple sources explain SCOPE` |
| Inventory declarations | `grepple --outline PATH` (verified local `@KIND:START-END` when available) |
| Retrieve enclosing implementation | `grepple -F 'Name' SCOPE --limit 5` |
| Locate exact evidence lines | `grepple --line-only -F 'Name' SCOPE` |
| Find the owner of a body match | `grepple --line-only --enclosing -F 'call()' file.go` |
| Retrieve a known literal range | `grepple --at path/to/file.go:START-END` |
| Verify an annotated scope, then read clean source | `grepple --at path/to/file.go @KIND:START-END` (local only) |
| Edit anchors | Eligible local source output uses native anchors; named compatibility providers come only from user settings |

`--purpose`, `--domain-area`, `--functional-area`, and `--keyword` apply to local `tree` and every local search output (including `--files`, `--count-summary`, `--outline`, and `--at`) **before** scanning/paging; repeated values OR within a category and categories AND on the same checksum-current file. Labels match exactly after lowercasing and turning spaces into hyphens; they are not relevance ranking. These selectors reject remote/mixed, cursor, piped stdin, and explicit related navigation; `graph`, `grit`, and `architecture` do not inherit them. Run `grepple verify` if a filtered result is unexpectedly empty; metadata must never silently broaden the query.

Line-only search may append `@if:3-5 @func:2-9` display hints; a human suffix can also occur in literal source and is **not** authoritative or edit-ready. `grepple --at PATH @KIND:START-END` (or `@KIND:LINE` when unambiguous) verifies the current parser-backed scope, or fails if stale, missing, or uncertain. For typed output use `--line-only --json-matches` (`text`, `nearest`, `scope`). `--files` lists paths, while `--files-with-matches` searches contents (`-l` lists paths). Results are in path order, not relevance order: an early page is not proof of completeness.

## Before a completeness claim

Start with `--count-summary` when scope is unknown. Check selected paths and exclusions with `grepple sources explain SCOPE` if they could matter; use `--production-only` only when tests and fixtures are intentionally excluded. In JSON check page completeness, omissions, and source diagnostics; use the supplied `nextCommand` to page. If output is a `grepple-artifact-v1` descriptor, inspect it before retrieving only relevant ranges. Recovered, failed, or unsupported parsing limits structural conclusions. A text occurrence is not a caller: use `grepple-change-impact-analysis` for that question.

## If a search command was blocked

| Attempt | Grepple starting point |
| --- | --- |
| `grep -rn`, `rg -n`, Claude Grep | `grepple --line-only -F 'text' PATH` (`-F` only for literals) |
| `grep -rl`, `rg -l` | `grepple -F 'text' --files-with-matches PATH` |
| `grep -c`, `rg -c` | `grepple -F 'text' --count PATH` (per-file matching-line counts; `--count-summary` is aggregate) |
| `find DIR -name '*.go'`, Claude Glob | `grepple --files '**/*.go' DIR` (eligible files, not directories) |

For ignored/generated/log or secret files, check eligibility with `grepple sources explain PATH` or read a known file directly; do not assume they are all searchable or recommend scanning `.env` by default. If Grepple cannot represent an exceptional **shell** search, append `# grep-guard:allow` to that Bash command; Claude's built-in Grep/Glob have no escape hatch.

## Before editing

Read clean source with `grepple --at PATH @KIND:START-END` (verified local scope) or `grepple --at PATH:START-END` (literal bounds). Copy fresh **three-character HASH** values, not line numbers or decorated search rows; re-anchor after edits or formatting. For inclusive replacements, snapshot rules, create/delete, and named markers, load `grepple-write` before writing.

Use `grepple-architecture-lookup-discovery` for ownership, `grepple-architecture-boundary-review` for proposed moves, `grepple-structural-pattern-audit` for syntax shapes, and `grepple-remote-grep-and-search` for indexed code outside this checkout.
