---
name: architecture-lookup-discovery
description: Use to orient in an unfamiliar local repository or answer directory ownership, declaration location, entrypoint, and source-linked cross-directory relation questions. Switch skills for call impact or proposed boundary changes.
---

# Architecture lookup with Grepple

## Choose the question

- **Owner unknown:** `grepple sources explain .`, then `grepple architecture directory --depth 2 --max-nodes 80 .`.
- **Symbol known:** `grepple --outline PATH` for a known file, or `grepple graph resolve --symbol NAME SCOPE` for an ambiguous callable. Verify the emitted location with `grepple --at PATH:LINE`.
- **Why directories relate:** `grepple architecture directory --relations SCOPE` and `grepple architecture why FROM TO SCOPE` for source-linked call/import/type evidence.

Follow one emitted `PATH:START-END` with `grepple --at PATH:START-END` to verify the declaration or relation. Report the owning directory, declaration kind/range, evidence confidence, and inspected source universe. Directory names indicate physical ownership, not package or runtime semantics.

## Stop or switch

Check coverage, exclusions, and truncation before claiming absence; unresolved/ambiguous or adapter-unsupported relations are not evidence of no dependency. Visibility and entrypoint semantics may be unsupported by a language adapter. Use `--production-only` only when tests are deliberately excluded. If JSON spills, inspect the `grepple-artifact-v1` descriptor or narrow the query.

Use `change-impact-analysis` for callers or refactor impact; use `architecture-boundary-review` before recommending a move or facade. If comparing two saved architecture reports for drift, run `grepple architecture compare before.json after.json` after checking they used the same source universe.
