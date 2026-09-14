# Next improvements

`solved.md` records completed capabilities. This file contains only open work and the evidence needed to prioritize it.

## Recommended next milestone

> Adopt one repository scope through the existing root `grepple.json`, replace and immediately remove Go-specific package/workspace orientation in favor of bounded directory architecture, and default large complete output to a disclosed file artifact with `--no-spill` as the compatibility escape hatch. Fix repository-origin classification before trusting boundary risk.

## Planning principles

- Prefer a new usable capability over endless syntax-resolution refinement after deterministic candidates, bounded output, and safe binding lifetime are established.
- Revisit resolution completeness when measured ambiguity blocks an actual task.
- Human output must remain bounded and disclose omissions. Complete machine output must remain available, but large results may be delivered through a disclosed file artifact rather than injected into stdout/context.
- Syntax-based navigation, structural matching, architecture relations, and boundary findings are evidence—not compiler, runtime, type-flow, or policy verdicts.
- Preserve dependency direction toward `api` and `parser`; keep GritQL query semantics out of `parser`.
- Keep canonical language IDs and grammar-kind policy out of generic navigation engines.

## P0 — restore source-scope and classification trust

### Repository identity hardening

Go navigation facts now receive path-dependent module/package identity after cache loading, and same-module imports classify as first-party before host-qualified third-party fallback.

- [ ] Add explicit nested-module, `go.work`, replacement, and ambiguous-unqualified-import fixtures.
- [ ] Add cold/warm and focused/full-universe parity coverage for enriched repository identity.

### Repository configuration and one source universe

Extend the existing repository-root `grepple.json` configuration, which already stores fields such as `server`. The first new capability should be ignore lists shared by search, graph, boundaries, extraction, GritQL scans, architecture discovery, and cache input discovery.

Proposed backward-compatible shape:

```json
{
  "server": "https://example.invalid",
  "ignore": {
    "paths": ["sandbox/**", "vendor/**", "generated/**"]
  },
  "output": {
    "spillThresholdBytes": 65536
  }
}
```

- [ ] Report the loaded config path/digest and excluded source totals by reason without making machine output cache-state dependent.
- [ ] Add `--no-repo-config`, `--no-config-ignore`, and an exclusion-explanation command or mode so hidden scope never becomes unexplained missing evidence; do not disable user authentication config when bypassing repository behavior.
- [ ] Report when an explicitly named file bypasses configured ignores; behavior is already consistent and covered across search and extraction discovery.
- [ ] Add production/test/generated/vendor classifications or a `--production-only` preset after the common ignore path is established; whole-repository recovery fixtures and unrelated sandboxes currently make completeness and resolution metrics unnecessarily pessimistic.

## P1 — simplify architecture discovery and output delivery

### Directory architecture instead of package/workspace bundles

The current package/workspace model is Go-specific and expensive to complete: the `search` overview is about 27 KB, structure 65 KB, and manifest 120 KB. A simple directory model can orient across all supported languages without inventing package semantics.

- [ ] Define one bounded, language-neutral directory projection: directory/file counts, detected languages, declaration-kind counts, public/exported surface where adapters know it, entrypoints, and source-linked import/reference edges.
- [ ] Add `grepple architecture directory [PATH]` with depth/node limits, deterministic collapse, explicit unknowns, and exact continuation commands.
- [ ] Add targeted architecture resolution for types, callables, routes, files, and directories so `Document` resolves directly instead of `graph resolve` parsing the repository and returning zero because it indexes callables only.
- [ ] Add a source-linked `architecture why FROM TO` query that explains the exact import/type/reference evidence behind a directory edge.
- [ ] Generate a compact normalized directory manifest usable for drift checks and targeted queries; do not require agents to read exhaustive Mermaid or a large raw manifest.
- [ ] Compare directory-manifest coverage and answer quality with current Go package/workspace bundles, then remove package/workspace commands, generation paths, tests, documentation, and canonical artifacts immediately rather than maintaining two orientation systems.

### Spill large output to a file artifact

Observed complete outputs reached about 1.55 MB for boundaries and 6.7 MB for a graph over only `parser search`. Valid uncapped JSON is useful for automation but is a severe agent-context trap. Spill mode should be the default above 64 KB; callers that require the original stdout stream can opt out explicitly.

- [ ] Add an explicit artifact path override without colliding with extraction's existing `--output` meaning.
- [ ] Teach agent skills to inspect the descriptor first and retrieve only relevant file ranges rather than loading the complete artifact.
- [ ] Benchmark artifact fallback by context bytes and retrieval turns, not only file-write runtime.

## P2 — improve precision and architectural signal

### Public syntax API

- [ ] Document distinct lifecycle and safety contracts for `Document`, `Node`, `DocumentView`, `ViewNode`, and `SyntaxNode`.
- [ ] Consolidate or deprecate overlapping surfaces when they do not provide measurable safety, ownership, or performance value.
- [ ] Review whether non-backend helpers still located in `parser/tree_sitter.go` belong with their semantic owner.

### Architecture diagnostics

- [ ] Compare normalized directory manifests before canonical byte comparison.
- [ ] Report the first changed declaration, member, route, relation, file, or directory with source-linked context before the final byte-level determinism comparison.
- [ ] Make command-specific errors explain focused-language versus language-neutral directory support; remove legacy Go-only bundle wording with the commands.

## Reliability and compatibility gates

- [ ] Add larger golden fixtures for every supported language.
- [ ] Cover malformed, nested, generic, decorated/annotated, and multiline declarations per language.
- [ ] Expand CRLF, symlink, build-constraint, ambiguous-extension, and Windows/cross-platform path cases.
- [ ] Run fuzz targets longer in scheduled CI and retain minimized failures as seeds.
- [ ] Verify generated and queried output determinism across every supported operating system.
- [ ] Add public `api` DTO compatibility tests against the private backend consumer when that checkout is available.
- Keep test, race, vet, lint, schema, architecture benchmark, and agent benchmark gates green after each change.

## Output and CLI consistency

- [ ] Reconcile or clearly document the different output-mode contracts: search human/JSON/JSON-matches, graph JSON-or-compact, boundaries human/JSON, extract Mermaid, and GritQL human/JSON.
- [ ] Clarify that `-l` lists paths rather than grep-style matching files; make `--files-with-matches` easy to discover.
- [ ] Explain JavaScript-regex defaults at first use and in examples.
- [ ] Make limit units explicit: files, findings, candidates per section, segments, nodes, bytes, and source files.
- [ ] Distinguish local and remote feature availability at the attempted command.
- [ ] Clarify that graph `dependencies`/`dependents` currently describe navigation edges rather than a build-system import graph.
- [ ] Separate resolution outcomes from confidence labels: report resolution, ambiguous-local, unresolved-local, and expected-external rates. The current `candidate` outcome and `candidate` confidence use different meanings, while ambiguity rate alone hides a large unresolved population.
- [ ] Keep complete JSON available for scripts, but direct agents toward filtered projections and artifact descriptors rather than multi-megabyte graph/boundary documents.
- [ ] Remove guidance drift such as duplicate workflow steps and completed milestones still named in the recommendation; add lightweight documentation consistency checks.

## Parser and cache direction

- [ ] Make parser documents the reusable cache boundary: a live Tree-sitter tree or versioned packed read-only CST behind one lightweight accessor.
- [ ] Parse or restore each file once, then derive outlines, navigation, extraction, and multiple GritQL evaluations.
- [ ] Keep packed per-file facts path-neutral; apply repository, module, directory, and configured-ignore context after loading.
- [ ] Invalidate by source digest, language, grammar fingerprint/ABI, schema, and parser/extractor version.

## Language expansion

- [ ] Define a useful non-class focused architecture contract for Rust from real questions before implementing it.
- [ ] Evaluate C/C++ projections only after Rust or separate dogfooding demonstrates a stable contract.
- Keep Shell focused extraction unsupported unless dogfooding defines a useful command/script projection.
- [ ] Add Swift, Ruby, PHP, or other grammars only after current capability parity, help discoverability, and cross-platform determinism gates are met.

## Success measures

- A new user can discover and run search, graph impact, boundaries, directory architecture, and one GritQL query using recursive CLI help only.
- An agent can move from omitted related edges to a complete focused query by copying one suggested command.
- Every bounded response or spilled artifact descriptor identifies the bound/delivery decision, omitted work, evaluated source universe, and completeness path.
- Boundary output separates third-party permeability from first-party reuse, standard-library spread, test-only use, and approved internal infrastructure.
- Warm repeated graph queries parse or restore every unchanged file at most once and exactly match cold output.
- Dogfood benchmarks measure answer correctness, false architectural claims, retrieval turns, stdout/context bytes, and artifact reads as well as runtime and allocations.
- Architectural recommendations preserve exact source evidence, confidence, scope, and unresolved alternatives instead of converting heuristics into facts.
