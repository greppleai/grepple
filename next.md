# Next improvements

`solved.md` records completed capabilities. This file contains only open work and the evidence needed to prioritize it.

## Recommended next milestone

> Strengthen boundary classification with repository-owned policy, clarify public syntax lifecycle contracts, and improve architecture drift diagnostics. Revisit navigation only when measured ambiguity identifies a blocked workflow.

## Planning principles

- Prefer a new usable capability over endless syntax-resolution refinement after deterministic candidates, bounded output, and safe binding lifetime are established.
- Revisit resolution completeness when measured ambiguity blocks an actual task.
- Human output must remain bounded and disclose omissions; JSON must remain valid and byte-uncapped, but paging/source limits must still be explicit.
- Syntax-based navigation, structural matching, architecture relations, and boundary findings are evidence—not compiler, runtime, type-flow, or policy verdicts.
- Preserve dependency direction toward `api` and `parser`; keep GritQL query semantics out of `parser`.
- Keep canonical language IDs and grammar-kind policy out of generic navigation engines.

## P2 — improve precision and architectural signal


### Public syntax API

- [ ] Document distinct lifecycle and safety contracts for `Document`, `Node`, `DocumentView`, `ViewNode`, and `SyntaxNode`.
- [ ] Consolidate or deprecate overlapping surfaces when they do not provide measurable safety, ownership, or performance value.
- [ ] Review whether non-backend helpers still located in `parser/tree_sitter.go` belong with their semantic owner.

### Architecture diagnostics

- [ ] Compare normalized manifests before canonical byte comparison.
- [ ] Report the first changed declaration, member, route, relation, package, or module with source-linked context before the final byte-level determinism comparison.
- [ ] Make command-specific errors explain focused-language versus Go-only canonical bundle support.

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

## Parser and cache direction

- [ ] Make parser documents the reusable cache boundary: a live Tree-sitter tree or versioned packed read-only CST behind one lightweight accessor.
- [ ] Parse or restore each file once, then derive outlines, navigation, extraction, and multiple GritQL evaluations.
- [ ] Keep packed per-file facts path-neutral; apply repository context after loading.
- [ ] Invalidate by source digest, language, grammar fingerprint/ABI, schema, and parser/extractor version.

## Language expansion

- [ ] Define a useful non-class focused architecture contract for Rust from real questions before implementing it.
- [ ] Evaluate C/C++ projections only after Rust or separate dogfooding demonstrates a stable contract.
- Keep Shell focused extraction unsupported unless dogfooding defines a useful command/script projection.
- [ ] Add Swift, Ruby, PHP, or other grammars only after current capability parity, help discoverability, and cross-platform determinism gates are met.

## Success measures

- A new user can discover and run search, graph impact, boundaries, focused extraction, and one GritQL query using recursive CLI help only.
- An agent can move from omitted related edges to a complete focused query by copying one suggested command.
- Every bounded response identifies the bound, omitted work, evaluated source universe, and completeness path.
- Boundary output separates third-party permeability from first-party reuse, standard-library spread, test-only use, and approved internal infrastructure.
- Warm repeated graph queries parse or restore every unchanged file at most once and exactly match cold output.
- Dogfood benchmarks measure answer correctness and retrieval turns as well as bytes, tokens, runtime, and allocations.
- Architectural recommendations preserve exact source evidence, confidence, scope, and unresolved alternatives instead of converting heuristics into facts.
