---
name: structural-pattern-audit
description: Use for local syntax-shaped audits where literal search would overmatch comments, strings, or unrelated syntax. GritQL detects shapes; it does not type-check, trace calls, or rewrite code.
---

# Structural pattern audit with Grepple GritQL

## Build and check one shape

1. Pick a supported language (`grepple languages`) and narrow source glob. For a Go call shape:
   ```bash
   grepple grit explain --json $'language go\n`exec.Command($args)`'
   grepple grit $'language go\n`exec.Command($args)`' 'internal/**/*.go'
   ```
   `explain` checks grammar wrapper and metavariable roles, **not** whether the query captures your intended policy.
2. Inspect positive and negative source examples. Add metavariables only for intended variation: repeating a named variable requires structural equality; `$_` matches independently. Add constraints after the basic shape works.
3. For a repository-wide compliance claim, run the narrowed query with `--limit 0 --json`, then check `resultMetadata.page.complete`, omissions, diagnostics, and selected files. `--limit 0` removes finding pagination, not scanner/resource truncation. If JSON spills, inspect its `grepple-artifact-v1` descriptor and relevant ranges. Zero findings mean “none” only when the intended files were eligible and diagnostics/truncation are empty.

The query must begin with `language ID`. Syntax matches ignore formatting/comments but preserve operators and literal spelling. `contains`/`within` include the node itself; unconstrained top-level `not` or absent `maybe` can match too broadly. For callers use `change-impact-analysis`; for ownership use `architecture-boundary-review`.
