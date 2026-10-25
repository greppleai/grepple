---
name: grepple-change-impact-analysis
description: Use before renaming, deleting, moving, or changing behavior/signatures of local code, or when asked what calls a symbol, what it calls, or what may break. Text matches alone are not impact evidence.
---

# Change-impact analysis with Grepple

## Trace one declaration

1. Locate the exact declaration. If its file is unknown and local metadata is current, search a promising subtree with `grepple --domain-area LABEL --line-only --enclosing -F 'Symbol' PATH` using a label observed in `overview` or a local tree; otherwise use `grepple --line-only --enclosing -F 'Symbol' SCOPE`. Widen without metadata selectors if coverage is incomplete or a filtered result is empty. For ambiguous callables, use `grepple graph resolve --symbol Symbol SCOPE`. Once the local file is known, prefer `grepple --outline PATH` and `grepple --at PATH @KIND:START-END` with the **emitted native token** for a clean, verified declaration read. An annotated `--line-only` hit may also supply a token, but the separate `--at` call must verify it before an edit; do not infer one from graph ranges or a literal source suffix. If no verified selector is available, read the exact literal span with `grepple --at PATH:LINE[-END]`. Go emits `@func`, TypeScript/JavaScript `@function` or `@method`. Neither a copied token nor this exact read proves call edges; graph selectors keep the legacy `PATH:LINE` form and do not accept metadata filters.
2. Preview immediate callers and callees with `grepple --related --at PATH:LINE`.
3. Choose the direction the change needs, over a universe that includes consumers outside the owning package:
   - Who calls it? `grepple graph callers --at PATH:LINE --depth 2 SCOPE`
   - What does it call? `grepple graph callees --at PATH:LINE --depth 2 SCOPE`
   - Bounded outgoing expansion from a source location? `grepple --follow-related 2 --at PATH:LINE` (1-3 outgoing call hops with direct callers). For a complete directional traversal, use `graph callers` or `graph callees` with `--depth 2`; `graph impact` is no longer available.
4. If bounded output omits results or completeness matters, narrow scope or rerun that focused query with `--json`. Follow `nextCommand` for paging; inspect spilled `grepple-artifact-v1` output by relevant range, not a repository-wide dump.

## Confidence and handoff

Treat `exact`, `import-resolved`, and safely `context-resolved` edges as static evidence. `unique-terminal` is inference, and `[candidate; try --at PATH:LINE]` requires inspection. Check selected/parsed/skipped/failed/recovered source totals and omissions before asserting “no callers.” `@KIND:START-END` only verifies **current local syntax bounds** for a clean read; it cannot certify runtime dispatch, a prior copy's bytes, or an exhaustive impact graph. Interface dispatch, reflection, generated calls, registration, and string lookup need separate checks when relevant.

Before editing, record the declaration, graph universe and source totals, confirmed direct callers/callees, ambiguous candidates, missing sources or omissions, and non-static mechanisms checked. Use architecture lookup for directory relations; a text occurrence or directory edge is not a callable dependency.
