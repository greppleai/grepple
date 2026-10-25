---
name: grepple-architecture-lookup-discovery
description: Use to orient in an unfamiliar local repository or answer directory ownership, declaration location, entrypoint, and source-linked cross-directory relation questions. Switch skills for call impact or proposed boundary changes.
---

# Architecture lookup with Grepple

Start with `grepple overview` in a local checkout to see coverage and grounded labels; when it reports missing/stale metadata, use unfiltered tree and source rather than an empty filtered result. Narrow a likely subtree using `grepple tree --purpose implementation --domain-area LABEL --depth 2 PATH` (replace LABEL with an observed label). Repeat a category for OR; categories AND on one checksum-current file. These filters do **not** scope `architecture directory` or graph queries.

## Choose the question

- **Owner unknown:** After `overview` and the top-level tree, use an observed domain-area or functional-area label with `grepple tree --domain-area LABEL --depth 2 PATH` to find a likely owner. Then run `grepple sources explain PATH` and `grepple architecture directory --depth 2 --max-nodes 80 PATH` on that physical scope. Metadata selectors do not apply to architecture; widen when coverage is incomplete or the owner is unclear.
- **Symbol known:** `grepple --outline PATH`; copy its emitted native token (Go `@func`, TypeScript `@function`/`@method`) and read with `grepple --at PATH @KIND:START-END` for a current, parser-verified local span. For ambiguous callables, use `grepple graph resolve --symbol NAME SCOPE` to find candidates, then inspect their local outlines; graph line ranges are not verified scope tokens.
- **Why directories relate:** `grepple architecture directory --relations SCOPE` for a compact map; use `grepple architecture directory --json SCOPE` for source-linked relation evidence, then inspect cited ranges with `grepple --at PATH:START-END`.

For rows without a token or for relation evidence, verify literal `PATH:START-END` with `grepple --at PATH:START-END`. Report the owning directory, declaration kind/range, evidence confidence, and inspected source universe. Directory names indicate physical ownership, not package or runtime semantics.

## Stop or switch

Check coverage, exclusions, and truncation before claiming absence; unresolved/ambiguous or adapter-unsupported relations are not evidence of no dependency. Visibility and entrypoint semantics may be unsupported by a language adapter. Use `--production-only` only when tests are deliberately excluded. If JSON spills, inspect the `grepple-artifact-v1` descriptor or narrow the query.

Use `grepple-change-impact-analysis` for callers or refactor impact; use `grepple-architecture-boundary-review` before recommending a move or facade. For drift between saved architecture reports, compare their JSON outputs with a JSON diff tool after checking they used the same source universe; `grepple architecture compare` is no longer available.
