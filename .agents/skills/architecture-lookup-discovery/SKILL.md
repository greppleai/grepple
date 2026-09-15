---
name: architecture-lookup-discovery
description: Use to orient in an unfamiliar local repository or answer directory ownership, declaration location, entrypoint, and source-linked cross-directory relation questions. Start from bounded directory architecture, then verify one exact source declaration. Do not use this skill alone for call impact or proposed boundary changes.
---

# Architecture lookup with Grepple

Directory architecture is a language-neutral orientation index, not proof of package semantics or runtime behavior. Use it to identify an owner and exact source range, then verify source.

## Fast path

1. **Unknown owner:** inspect the source universe, then request a bounded directory map.
   ```bash
   grepple sources explain --compact .
   grepple architecture directory --depth 2 --max-nodes 80 --compact .
   ```
2. **Known symbol:** resolve it directly across types and callables.
   ```bash
   grepple architecture resolve --symbol Document --compact .
   ```
   Retain every emitted match when the name is ambiguous; narrow PATH instead of guessing.
3. **Why two directories are related:** request exact call/import/type evidence.
   ```bash
   grepple architecture why rulespec search --compact rulespec search
   ```
4. **Verify source:** follow the emitted `PATH:START-END`.
   ```bash
   grepple --at path/to/file.go:40-80
   ```
5. **Only if behavioral impact matters:** switch to `change-impact-analysis` instead of inferring complete behavior from a directory relation.
6. **Diagnose unexpected architecture drift:** compare complete reports from the same intended source universe before inspecting checksums.
   ```bash
   grepple architecture compare --compact before.json after.json
   ```
   A semantic difference identifies the first source-linked fact. Semantic equality with byte inequality identifies encoding or ordering drift.

## Trust rules

- Directory names establish physical ownership, not language package/module/layer intent.
- Directory `why` relations distinguish strongly resolved static calls, adapter-owned imports, and imported type references. Inspect relation coverage: ambiguous, unresolved, unqualified, and adapter-unsupported facts are not asserted as edges, so absence is not proof that no reflection, registration, build-system, or runtime dependency exists.
- Declaration visibility, entrypoints, and routes are reported only when an owning language adapter provides the corresponding contract; unsupported semantics must stay unknown. Route registration is syntax evidence, not proof that a server starts or the route is reachable at runtime.
- Inspect `sources` and truncation before making a completeness claim. Use `grepple sources explain --compact PATH` when skipped input or repository configuration could matter; use `--production-only` only when the question is explicitly about production code.
- Large JSON may be returned as a `grepple-artifact-v1` descriptor. Read only relevant artifact ranges or rerun a narrower command; use the descriptor's exact `--no-spill` command only when the complete stdout stream is required.
- Do not invent a package boundary from directory names alone.

## Report only evidence

Preserve the owning directory, language, declaration kind/range, relation confidence, evaluated source universe, and next source location inspected. State uncertainty rather than filling missing semantics from the directory layout.

Use `architecture-boundary-review` when deciding whether code should move, split, or sit behind a facade.
