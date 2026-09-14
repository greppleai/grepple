---
name: structural-pattern-audit
description: "Use for local syntax-shaped audits: finding API call forms across formatting variants, repeated code structures, forbidden constructs, migration candidates, or repository policy violations. Uses native read-only Grepple GritQL when literal/regex search would overmatch comments, strings, or unrelated syntax."
---

# Structural pattern audit with Grepple GritQL

Use text search for names; use GritQL when syntax position and shape determine correctness. Structural findings reduce false positives, but they do not provide type checking or data flow.

## Workflow

1. Identify the target language and narrow source glob.
2. Start with one concrete positive shape:
   ```bash
   grepple grit $'language go\n`exec.Command($args)`' 'internal/**/*.go'
   ```
3. Use metavariables only where variation is intended. Repeating a named metavariable requires structural equality; `$_` is an independent anonymous wildcard.
4. Add constraints only after the basic shape matches:
   ```bash
   grepple grit $'language go\n`exec.Command($args)` where { $args <: r"^ctx," }' 'internal/**/*.go'
   ```
5. Remove finding pagination and use JSON before making a repository-wide compliance claim:
   ```bash
   grepple grit --limit 0 --json $'language go\n`exec.Command($args)`' 'internal/**/*.go'
   ```
   JSON avoids byte truncation; `--limit 0` removes the default 20-finding page. Check `resultMetadata.page.complete`, omissions, diagnostics, and any copyable `nextCommand`; scanner/resource truncations can still make the evaluated source universe incomplete.
6. Inspect representative positive and negative source ranges. A zero-result query is trustworthy only when diagnostics and truncation records are empty and the intended files were eligible.

## Safety and precision rules

- The first line must be exactly `language ID`; supported IDs are shown by `grepple languages`.
- Snippets are structural: formatting/comments are ignored, punctuation/operators/literal spelling remain significant.
- GritQL is detection-only. It does not resolve names or types, follow calls across files, prove data flow, or rewrite source.
- `contains`/`within` are reflexive; do not assume strict descendant/ancestor semantics.
- Top-level `not` or absent `maybe` can match many candidates; normally place them inside an `and` or constraint.
- Source parse or resource diagnostics make the affected scope incomplete. Never report “none exist” while diagnostics or truncations remain.
- Use `change-impact-analysis` for callers and `architecture-boundary-review` for ownership; do not stretch a syntax query into those claims.

## Why this prevents bad decisions

Literal search can match comments, strings, declarations, and unrelated receivers. A structural query proves that the returned bytes occupy the requested grammar shape. Verification of sample ranges then catches a query that is valid but expresses the wrong policy.
