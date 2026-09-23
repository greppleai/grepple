---
name: change-impact-analysis
description: Use before renaming, deleting, moving, or changing behavior/signatures of local code, or when asked what calls a symbol, what it calls, or what may break. Uses Grepple navigation and graph queries; text occurrences alone are not impact evidence.
---

# Change-impact analysis with Grepple

Prevent the two common mistakes: treating every name occurrence as a caller, and treating a bounded navigation preview as a complete call graph.

## Workflow

1. Resolve the exact declaration location. Prefer an existing range; otherwise locate it:
   ```bash
grepple --line-only --enclosing -F 'Symbol' SCOPE
grepple graph resolve --symbol Symbol SCOPE
grepple --at path/to/file.go:LINE
   ```
   Use `graph resolve` when a name is overloaded or appears in multiple containers; continue with one emitted `--at` selector rather than guessing.
2. Preview immediate behavior and consumers:
   ```bash
   grepple --related --at path/to/file.go:LINE
   ```
3. For a refactor decision, query the needed direction over the relevant source universe:
   ```bash
   grepple graph callers --at path/to/file.go:LINE --depth 2 SCOPE
   grepple graph callees --at path/to/file.go:LINE --depth 2 SCOPE
   grepple graph impact  --at path/to/file.go:LINE --depth 2 SCOPE
   ```
4. If compact output reports omissions or completeness is required, narrow the universe or rerun the focused query with `--json`; do not replace it with a whole-repository graph dump. If JSON spills, inspect the `grepple-artifact-v1` descriptor and read only relevant artifact ranges. Use resolution totals and per-language/confidence ambiguity rates to decide whether candidate inspection is material for this scope.

## Confidence rules

- `exact`, `import-resolved`, and safely `context-resolved` edges are evidence.
- `unique-terminal` is syntax-based inference, not type checking.
- `[candidate; try --at PATH:LINE]` is a lead. Inspect candidates before choosing one.
- Paths passed to the command define the graph universe. Include consumers outside the declaration's package when claiming repository impact.
- Check `metadata.page.complete` plus discovered/selected/parsed/skipped/failed/recovered source totals. Failed, recovered, or truncated source prevents a complete static-impact claim; use `nextCommand` when supplied.
- Navigation does not prove interface dispatch, reflection, generated calls, runtime registration, data flow, or string-based lookup. Search those mechanisms explicitly when relevant.
- Directory relation evidence and callable impact differ. Use `architecture directory|resolve|why` for physical ownership and source-linked call/import/type relations, but use graph traversal for callable impact and inspect architecture coverage before treating an absent relation as evidence.

## Decision record

Before changing code, state: exact declaration, graph universe and source totals, direct callers, direct callees, ambiguous candidates, omitted counts, and non-static mechanisms checked. This makes “no callers” a scoped evidence claim instead of a guess.
