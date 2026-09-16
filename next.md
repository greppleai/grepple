# Next improvements

`solved.md` records completed capabilities. This file contains only open work and the evidence needed to prioritize it.

## Recommended next milestone

> Verify generated and queried output determinism across supported operating systems, using normalized architecture comparison to localize drift.

## Planning principles

- Prefer a new usable capability over endless syntax-resolution refinement after deterministic candidates, bounded output, and safe binding lifetime are established.
- Revisit resolution completeness when measured ambiguity blocks an actual task.
- Human output must remain bounded and disclose omissions. Complete machine output must remain available, but large results may be delivered through a disclosed file artifact rather than injected into stdout/context.
- Syntax-based navigation, structural matching, architecture relations, and boundary findings are evidence—not compiler, runtime, type-flow, or policy verdicts.
- Preserve dependency direction toward `api` and `parser`; keep GritQL query semantics out of `parser`.
- Keep canonical language IDs and grammar-kind policy out of generic navigation engines.

## Reliability and compatibility gates

- [ ] Add larger golden fixtures for every supported language.
- [ ] Cover malformed, nested, generic, decorated/annotated, and multiline declarations per language.
- [ ] Expand CRLF, symlink, build-constraint, ambiguous-extension, and Windows/cross-platform path cases.
- [ ] Run fuzz targets longer in scheduled CI and retain minimized failures as seeds.
- [ ] Verify generated and queried output determinism on Linux, macOS, and Windows; compare normalized semantics first and raw bytes second, retaining the first source-linked failure as CI evidence.
- [ ] Define schema compatibility and deprecation rules for public `api` DTOs and versioned CLI JSON before the next public release.
- Keep test, race, vet, lint, schema, architecture benchmark, and agent benchmark gates green after each change.

### Delegated research performance

Live `grepple ask` dogfooding showed two distinct bottlenecks: ordinary local/remote investigations took about 55 seconds while tool execution consumed only 0.85–3.09 seconds, so model round trips dominated; a subprocess audit spent 41.58 of 60.11 seconds in tools because two unnecessary whole-repository navigation builds took 14.13 and 27.15 seconds.

- [ ] Add a focused `inspect_symbol` tool that can combine bounded symbol discovery, deterministic declaration selection, immediate related navigation, and exact source retrieval in one call, reducing common count → files → snippets → navigate → read sequences.
- [ ] Add an `auto` `search_code` projection that returns aggregate concentration, top matching paths, bounded snippets, and exact next locations together while retaining explicit `count`, `files`, and `snippets` modes.
- [ ] Detect repeated or increasingly broad equivalent research calls and return cached evidence plus a synthesis/narrowing hint. Do not reintroduce a model-step cutoff; the overall timeout remains the execution bound.
- [ ] Reduce repeated model-input tokens by activating only the relevant typed-tool subset after initial intent becomes observable, while ensuring local, remote, architecture, and exact-location questions can still reach every required tool.
- [ ] Add parsed-versus-restored file counts and universe build/restore durations to ask performance logs once parser-document restoration is implemented; keep token chunks omitted.
- [ ] Add answer-gated ask benchmarks for local ownership, subprocess/security audit, graph impact, structural matching, and exact remote-version research. Gate on answer correctness, citation validity, ref fidelity, completeness disclosure, elapsed time, model steps, tool calls, tool time, and tokens rather than latency alone.
- [ ] Set regression targets from the live baseline: eliminate full-graph work for invalid navigation locations, avoid repeated identical scans within one ask, and reduce routine source-backed investigations from multi-call discovery chains to one or two evidence calls before synthesis.

### Navigation and evidence calibration

- [ ] Publish per-language coverage for declarations, calls, imports, type references, fields, member access, and process entrypoints; distinguish unsupported facts from unresolved facts.
- [ ] Add adapter-owned import-relation parity for other supported languages where syntax and repository layout permit conservative local target resolution.
- [ ] Dogfood scoped Rust module identities on representative crates; add raw-string `#[path]` support and visibility-aware access checks without evaluating conditional `cfg` expressions or guessing external-crate ownership.
- [ ] Add cross-file navigation goldens for aliases, receivers, overload-like declarations, generics, nested scopes, and ambiguous imports in every language that claims the relevant adapter capability.
- [ ] Measure resolved-local, ambiguous-local, unresolved-local, and expected-external outcomes on representative repositories instead of treating candidate counts as precision.
- [ ] Add answer-gated tasks that intentionally exercise reflection, dynamic dispatch, generated code, registration, and build-system edges; verify Grepple reports uncertainty rather than unsupported architectural claims.
- [ ] Document when users must hand evidence to a compiler, language server, build-system query, or runtime tool for refactor safety; do not imply Grepple alone proves exact semantic impact.

### Structural-query reliability

- [ ] Expand `gritql-v1` conformance fixtures across every supported language for named/list metavariables, ambiguous snippet contexts, malformed syntax, cancellation, and resource limits.
- [ ] Benchmark structural scans on representative medium and large repositories, including peak memory, cancellation latency, and cold/warm behavior.

## Output and CLI consistency

- [ ] Reconcile or clearly document the different output-mode contracts: search human/JSON/JSON-matches, graph JSON-or-compact, boundaries human/JSON, extract Mermaid, and GritQL human/JSON.
- [ ] Explain JavaScript-regex defaults at first use and in examples.
- [ ] Make limit units explicit: files, findings, candidates per section, segments, nodes, bytes, and source files.
- [ ] Distinguish local and remote feature availability at the attempted command.
- [ ] Clarify that graph `dependencies`/`dependents` currently describe navigation edges rather than a build-system import graph.
- [ ] Separate resolution outcomes from confidence labels: report resolution, ambiguous-local, unresolved-local, and expected-external rates. The current `candidate` outcome and `candidate` confidence use different meanings, while ambiguity rate alone hides a large unresolved population.
- [ ] Keep complete JSON available for scripts, but direct agents toward filtered projections and artifact descriptors rather than multi-megabyte graph/boundary documents.
- [ ] Remove guidance drift such as duplicate workflow steps and completed milestones still named in the recommendation; add lightweight documentation consistency checks.

## Operational and release readiness

- [ ] Run the `v0.0.1` Release Please flow on GitHub and verify all five native hosted-runner archives, checksums, conditional attestation behavior, and downloaded-binary smoke tests; extend smoke coverage beyond `version` to recursive help, local search, architecture comparison, and one GritQL query.
- [ ] Test concurrent processes writing the same content-addressed artifact or cache entry, interrupted writes, corrupt entries, cleanup during reads, and permission preservation.
- [ ] Measure cold/warm runtime, peak RSS, cache size, artifact disk growth, and cleanup behavior on representative medium and large repositories.
- [ ] Define supported Go versions, operating systems, repository-size expectations, schema support windows, and release rollback/migration behavior.
- [ ] Add explicit local-only, authentication-required, backend-unavailable, and unsupported-remote command tests so deployment availability is observable rather than inferred.

## Parser and cache direction

- [ ] Make parser documents the reusable cache boundary: a live Tree-sitter tree or versioned packed read-only CST behind one lightweight accessor.
- [ ] Parse or restore each file once, then derive outlines, navigation, extraction, and multiple GritQL evaluations.
- [ ] Keep packed per-file facts path-neutral; apply repository, module, directory, and configured-ignore context after loading.
- [ ] Invalidate by source digest, language, grammar fingerprint/ABI, schema, and parser/extractor version.

## Language expansion

- [ ] Evaluate C/C++ projections only after Rust dogfooding demonstrates a stable contract.
- Keep Shell focused extraction unsupported unless dogfooding defines a useful command/script projection.
- [ ] Add Swift, Ruby, PHP, or other grammars only after current capability parity, help discoverability, and cross-platform determinism gates are met.

## Success measures

- A new user can discover and run search, graph impact, boundaries, directory architecture, and one GritQL query using recursive CLI help only.
- An agent can move from omitted related edges to a complete focused query by copying one suggested command.
- Every bounded response or spilled artifact descriptor identifies the bound/delivery decision, omitted work, evaluated source universe, and completeness path.
- Boundary output separates third-party permeability from first-party reuse, standard-library spread, test-only use, and approved internal infrastructure.
- Warm repeated graph queries parse or restore every unchanged file at most once and exactly match cold output.
- Dogfood benchmarks measure answer correctness, false architectural claims, retrieval turns, stdout/context bytes, artifact reads, peak memory, and cold/warm cache behavior as well as runtime and allocations.
- Per-language capability reports distinguish unsupported analysis from attempted-but-unresolved evidence.
- The same complete report generated on Linux, macOS, and Windows is normalized-semantically equal; byte differences either fail the gate or have an explicit documented platform reason.
- Public DTO and CLI JSON changes have consumer compatibility evidence and a declared migration path.
- Architectural recommendations preserve exact source evidence, confidence, scope, and unresolved alternatives instead of converting heuristics into facts.
