---
name: architecture-lookup-discovery
description: Use to orient in an unfamiliar local repository or answer directory ownership, declaration location, entrypoint, and source-linked cross-directory relation questions. Start from bounded directory architecture, then verify one exact source declaration. Do not use this skill alone for call impact or proposed boundary changes.
---

# Architecture lookup with Grepple

Directory architecture is a language-neutral orientation index, not proof of package semantics or runtime behavior. Use it to identify an owner and exact source range, then verify source.

## Fast path

1. **Unknown owner:** inspect the source universe, then request a bounded directory map.
   ```bash
   grepple sources explain .
   grepple architecture directory --depth 2 --max-nodes 80 .
   ```
2. **Known symbol:** use `grepple --outline PATH`, or `grepple graph resolve --symbol NAME SCOPE` for an ambiguous callable. Verify one emitted location with `grepple --at PATH:LINE`.
3. **Why two directories are related:** scope `grepple architecture directory --relations FROM TO`, then inspect the source-linked relation evidence in its `--json` output or search exact imports/calls. A static relation does not prove runtime behavior.
4. **Behavioral impact:** switch to `change-impact-analysis` and focused `graph callers|callees`.

## Trust rules

- Directory names establish physical ownership, not language package/module/layer intent.
- Directory relations distinguish syntax-resolved calls, imports, and type references. Inspect coverage; ambiguity, unresolved inputs, and adapter-unsupported facts make absence inconclusive.
- Declaration visibility and process entrypoints are reported only when an owning language adapter provides the corresponding contract; unsupported semantics must stay unknown.
- Inspect `sources` and truncation before making a completeness claim. Use `grepple sources explain PATH` when skipped input or repository configuration could matter; use `--production-only` only when the question is explicitly about production code.
- Large JSON may be returned as a `grepple-artifact-v1` descriptor. Read only relevant artifact ranges or rerun a narrower command; use the descriptor's exact `--no-spill` command only when the complete stdout stream is required.
- Do not invent a package boundary from directory names alone.

## Report only evidence

Preserve the owning directory, language, declaration kind/range, relation confidence, evaluated source universe, and next source location inspected. State uncertainty rather than filling missing semantics from the directory layout.

Use `architecture-boundary-review` when deciding whether code should move, split, or sit behind a facade.
