---
name: grepple-architecture-lookup-discovery
description: Use to orient in an unfamiliar local repository or answer directory ownership, declaration location, entrypoint, and source-linked cross-directory relation questions. Switch skills for call impact or proposed boundary changes.
---

# Architecture lookup with Grepple

## Choose the question

- **Owner unknown:** `grepple sources explain .`, then `grepple architecture directory --depth 2 --max-nodes 80 .`.
- **Symbol known:** `grepple --outline PATH`; follow a verified token with `grepple --at PATH @KIND:START-END`. For ambiguous callables, use `grepple graph resolve --symbol NAME SCOPE`.
- **Why directories relate:** `grepple architecture directory --relations SCOPE` for a compact map; use `grepple architecture directory --json SCOPE` for source-linked relation evidence, then inspect cited ranges with `grepple --at PATH:START-END`.

For rows without a token or for relation evidence, verify literal `PATH:START-END` with `grepple --at PATH:START-END`. Report the owning directory, declaration kind/range, evidence confidence, and inspected source universe. Directory names indicate physical ownership, not package or runtime semantics.

## Stop or switch

Check coverage, exclusions, and truncation before claiming absence; unresolved/ambiguous or adapter-unsupported relations are not evidence of no dependency. Visibility and entrypoint semantics may be unsupported by a language adapter. Use `--production-only` only when tests are deliberately excluded. If JSON spills, inspect the `grepple-artifact-v1` descriptor or narrow the query.

Use `grepple-change-impact-analysis` for callers or refactor impact; use `grepple-architecture-boundary-review` before recommending a move or facade. For drift between saved architecture reports, compare their JSON outputs with a JSON diff tool after checking they used the same source universe; `grepple architecture compare` is no longer available.
