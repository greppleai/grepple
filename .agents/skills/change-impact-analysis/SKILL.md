---
name: change-impact-analysis
description: Use before renaming, deleting, moving, or changing behavior/signatures of local code, or when asked what calls a symbol, what it calls, or what may break. Text matches alone are not impact evidence.
---

# Change-impact analysis with Grepple

## Trace one declaration

1. Resolve the exact declaration: `grepple graph resolve --symbol Symbol SCOPE` if overloaded, or locate it with `grepple --line-only --enclosing -F 'Symbol' SCOPE`. Verify the chosen `PATH:LINE` with `grepple --at PATH:LINE`.
2. Preview immediate callers and callees with `grepple --related --at PATH:LINE`.
3. Choose the direction the change needs, over a universe that includes consumers outside the owning package:
   - Who calls it? `grepple graph callers --at PATH:LINE --depth 2 SCOPE`
   - What does it call? `grepple graph callees --at PATH:LINE --depth 2 SCOPE`
   - Multi-hop change reach? `grepple graph impact --at PATH:LINE --depth 2 SCOPE`
4. If bounded output omits results or completeness matters, narrow scope or rerun that focused query with `--json`. Follow `nextCommand` for paging; inspect spilled `grepple-artifact-v1` output by relevant range, not a repository-wide dump.

## Confidence and handoff

Treat `exact`, `import-resolved`, and safely `context-resolved` edges as static evidence. `unique-terminal` is inference, and `[candidate; try --at PATH:LINE]` requires inspection. Check selected/parsed/skipped/failed/recovered source totals and omissions before asserting “no callers.” Interface dispatch, reflection, generated calls, registration, and string lookup need separate checks when relevant.

Before editing, record the declaration, graph universe and source totals, confirmed direct callers/callees, ambiguous candidates, missing sources or omissions, and non-static mechanisms checked. Use architecture lookup for directory relations; a text occurrence or directory edge is not a callable dependency.
