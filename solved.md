# Solved improvements

This file records completed Grepple roadmap work that no longer belongs in `next.md`. It is a capability history, not a list of current priorities.

## Agent workflows and guidance

- [x] Proved the workspace summary → package summary → exact `--at` → bounded `--related` workflow for unfamiliar repositories.
- [x] Added short intent-triggered skills for local retrieval, architecture lookup, change impact, boundary review, structural audits, architecture diagrams, and remote inspection.
- [x] Made each skill state what Grepple evidence can and cannot prove, with explicit handling for candidates, bounded output, source universes, and syntax-only analysis.
- [x] Added repeatable agent workflow benchmarks for breadth summaries, outlines, structural lookup, exact ranges, related navigation, impact graphs, enclosing scopes, and edit locations.

## CLI discoverability

- [x] Added top-level help that lists search, GritQL, graph, boundaries, extraction, language capabilities, rules, remote repository/authentication commands, and version before the default search options.
- [x] Added real `grepple help` and command help routing plus an explicit `grepple search` mode so command-name patterns do not collide with dispatch.
- [x] Added successful recursive help for every extract and graph mode, including explicit output-mode and root-selector exclusivity contracts.
- [x] Added `grepple examples [TASK]` with concise copyable workflows for orientation, exact retrieval, editing, caller impact, boundary review, GritQL audits, focused diagrams, and canonical checks.
## Repository scope and output delivery

- [x] Extended the nearest ancestor root `grepple.json` with validated repository-relative `ignore.paths` and `output.spillThresholdBytes` while preserving existing `server` files.
- [x] Kept authentication user-owned: repository configuration cannot provide or override access tokens, refresh tokens, expiries, or user identity.
- [x] Applied one ordered `**`/negation-capable ignore matcher to search, graph, boundaries, GritQL, and focused extraction; recursive discovery honors ignores while explicitly named files bypass them.
- [x] Excluded `.grepple/cache/` and `.grepple/output/` from recursive source discovery and Git.
- [x] Added default 64 KiB output spilling to content-addressed mode-0600 artifacts with valid human/JSON descriptors, original schema/source metadata, exact `--no-spill` reruns, and threshold overrides.
- [x] Added `grepple artifacts clean` for explicit deterministic artifact cleanup.
- [x] Replaced process-exiting library paths with propagated command exit statuses so output delivery is finalized before grep-style exit code 1.

## Search and retrieval

- [x] Added `--count-summary` for complete matched-file and matching-line breadth independent of output paging.
- [x] Kept grep-compatible per-file `--count` and grouped `--count-by-repo` output.
- [x] Added safe compatibility aliases: `-E` selects JavaScript regex and `-r` is a recursive-search no-op.
- [x] Bounded human-readable output at 16,384 bytes while keeping JSON valid and byte-uncapped.
- [x] Reported omitted segments and broad-output truncation with narrowing guidance; short whitespace-only gaps render as whitespace instead of verbose collapsed-line markers.
- [x] Added parser-backed construct ranges to `--line-only` and opt-in enclosing ranges without changing anchor rows.
- [x] Preserved exact `HASH│LINE│content` output through a user-owned, versioned anchor-provider protocol.
- [x] Added provider digest checks, timeout/output bounds, strict response validation, default enablement, and `--no-anchors` fallback.
- [x] Added `anchors doctor` human/JSON diagnostics for settings and provider identity, executable availability, protocol version, timeout/output limits, and a validated temporary-file round trip with actionable setup guidance.
- [x] Added explicit fresh-environment `anchors setup`: preview-only by default, atomic mode-0600 user-settings writes behind `--write`, replacement behind `--force`, and no repository configuration or provider execution during setup.
- [x] Streamed validated focused extraction to stdout when no output path is supplied.
- [x] Classified default structural-search results as structured, recovered, plain, unsupported, or failed; incomplete human output is flagged and complete JSON carries per-file plus aggregate status.
- [x] Standardized search, graph, graph diff/resolve/query, boundary, and CLI GritQL result metadata for normalized scope, paging completeness, effective user caps, known omissions, stable diagnostics, and shell-quoted continuation commands.
- [x] Documented exact structural segment selection and added opt-in `--sort matches`: matching-line count descending with repository/path tie-breakers, full-universe scan semantics, wire support, and deterministic local/merged paging.

## Shared navigation graph

- [x] Added one normalized `parser.NavigationGraph` with stable declarations, calls, candidates, source ranges, confidence, visibility, package/module/container context, imports, receivers, return types, type usages, and member accesses.
- [x] Made `--at`, `--related`, focused flow generation/validation, JSON, compact output, graph queries, semantic diffs, and parity tests consume the shared projection.
- [x] Preserved `Symbol.Calls` and `Symbol.CallOrder` as compatibility projections instead of independent call collectors.
- [x] Added deterministic ambiguity ranking and actionable `--at PATH:LINE` candidate suggestions.
- [x] Added exact, import-resolved, context-resolved, unique-terminal, and candidate confidence classes.
- [x] Resolved explicit Go package imports and TypeScript named/namespace imports before terminal fallback.
- [x] Added direct receiver/parameter context, source-ordered lexical bindings, local/imported return inference, and typed member chains.
- [x] Added package/module-aware cross-file field facts, embedded/promoted Go method resolution, and TypeScript/TSX inheritance resolution while preserving conflicting candidates.
- [x] Added TypeScript/TSX named/default aliases, relative barrel re-exports, member chains, and nearest-`tsconfig.json` `baseUrl`/`paths` resolution with JSONC comment/trailing-comma support.
- [x] Added deterministic ambiguity fixtures for interfaces, overloads, inheritance, re-exports, default imports, aliases, and TSX path aliases.
- [x] Reported omitted callers and callees in human and JSON related output.
- [x] Made every human omitted-edge notice provide a copyable focused `graph callers|callees|impact --at PATH:LINE --depth 2 --json .` continuation.
- [x] Added complete `grepple-navigation-graph-v1` JSON and bounded compact graph projections.
- [x] Added deterministic discovered, selected, parsed, skipped, failed, recovered, and truncated source accounting to graphs, focused graph queries, graph diffs, and boundary reports.
- [x] Added graph-wide and per-language/confidence resolution measurements, including resolved, ambiguous, unresolved, singleton-candidate counts, and ambiguity frequency in complete JSON plus compact aggregate headers.
- [x] Added parser-owned content- and grammar-addressed per-file navigation facts shared by graph output/queries, `--related`, boundaries, and extraction, with atomic corruption-tolerant storage, path instantiation, explicit bypass, cold/warm parity tests, and a repeatable benchmark.
- [x] Enriched path-neutral cached Go facts with owning module/package identity after loading so host-qualified same-module imports classify as first-party instead of critical third-party leakage.
- [x] Added callers, callees, dependencies, dependents, and bidirectional impact queries with depth bounds, cycle safety, candidate preservation, and exact location/symbol/package/module/path roots.
- [x] Added traversal-free `graph resolve --symbol NAME` previews with deterministic exact/terminal matches, full stable IDs, exact `--at` alternatives, filters, and copyable focused graph commands in bounded compact and complete JSON output.
- [x] Added language, confidence, and visibility filtering before root selection and traversal.
- [x] Added `grepple-navigation-diff-v1`, ignoring line-only movement while reporting semantic declaration and call-edge changes.
- [x] Added end-to-end shared-edge parity coverage across JSON, compact graph output, focused Mermaid, directory resolution, and `--related`.

## Architecture extraction and validation

- [x] Replaced hook-local Mermaid extraction/checking with the shared root `extract` implementation.
- [x] Added source-linked, deterministic structure and flow generation with bounded depth/node counts.
- [x] Added explicit, checker-valid truncation markers to bounded diagrams.
- [x] Added structure and flow checking.
- [x] Previously implemented and measured canonical Go package/workspace bundles and bounded summaries; later removed them after dogfooding showed that a simpler directory projection generalized across languages with lower workflow complexity.
- [x] Added exact declaration/member ranges and deterministic relation evidence to generated focused diagrams.
- [x] Added focused structure/flow support for Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, and C#.
- [x] Added semantic navigation graph diffing for code review and CI impact.
- [x] Added `grepple architecture directory` with bounded hierarchical directory/file/language/declaration counts across every navigation-backed language.
- [x] Added `architecture resolve` for direct type/callable ownership and exact ranges, including declarations omitted by callable-only `graph resolve`.
- [x] Added `architecture why` with source-linked cross-directory call evidence restricted to exact/import/context-resolved edges.
- [x] Removed Go-specific package/workspace commands, public capability fields, generators, checkers, hook paths, tests, and committed `.grepple` artifacts without a compatibility period.

## Parser and language architecture

- [x] Moved application-source parsing behind `parser.Document`, callback-scoped `DocumentView`, and shared traversal/range APIs.
- [x] Removed extraction and hook parser construction, grammar selection, and duplicate application-source parser lifecycles.
- [x] Added a parser-owned language capability registry with canonical IDs, extensions, grammar ABI/fingerprints, navigation support, and generated grammar cardinality/subtype metadata.
- [x] Added iterative bounded syntax traversal and immutable subtree snapshot reuse within document views.
- [x] Moved callable, declaration, call, container, import, binding, return, visibility, receiver, member, and field syntax policy behind `languageAdapter.Navigation()`.
- [x] Added architecture guards preventing language-ID and mixed grammar-kind policy from returning to generic navigation engines.
- [x] Routed parsing through `languageAdapter.Parse()` and hid raw Tree-sitter grammars, trees, nodes, and cursors behind private syntax handles.
- [x] Removed the public raw-tree graph API and prevented production go-tree-sitter imports outside approved syntax backends.
- [x] Preserved exact source ranges, malformed-source fallback, and generated artifact behavior through the refactor.
- [x] Kept public package dependencies directed toward `api` and `parser`; the public module does not depend on private backend code.

## Native structural querying

- [x] Added one native, bounded, read-only `gritql-v1` contract.
- [x] Kept query parsing, algebra, bindings, matching, constraints, transactions, limits, and diagnostics in `gritql` rather than `parser`.
- [x] Added GritQL target adapters for Go, JavaScript/JSX, TypeScript/TSX, Python, C, C++, C#, Java, Kotlin, Rust, and Shell.
- [x] Added grammar-local snippet wrappers, expression/type/statement/declaration/sequence contexts, repeated-list matching, malformed-source handling, and binding-equality fixtures.
- [x] Added mixed-language saved-rule batching that reads/parses each eligible source once per matching language group.
- [x] Rejected unsupported languages, syntax, and compatibility mismatches without text or parser fallback.
- [x] Unified cancellable source discovery, ignore/glob handling, bounded acquisition, binary checks, mandatory anchors, and deterministic ordering with local search infrastructure.
- [x] Ensured anchored scans do not reread selected source before structural evaluation.
- [x] Added bounded compile-only `grit explain` human/JSON output for target language, compatibility, grammar identity, used features, wrapper interpretations, node/list metavariable roles, occurrence ranges, and stable compile diagnostics.
- [x] Expanded grammar-local TypeScript/TSX roles and wrappers for default-import identifiers, module sources, JSX identifier positions, class members, and declaration alternatives without fallback parsing.

## Boundary analysis

- [x] Added `grepple boundaries [PATH...]` over the shared navigation graph.
- [x] Added file-owned workflow signals for external consumer breadth, callable surface, callable co-usage, ordered sequences, and member-read/write plus call combinations.
- [x] Excluded owner-local calls and required repeated evidence across at least two external files.
- [x] Added import-qualified and unambiguously owned project type spread with parameter/result/receiver/local roles, public signature exposure, and production/test reach.
- [x] Classified type origin independently as local, first-party, standard-library, third-party, or unresolved while retaining imported identity and the compatibility `external` signal.
- [x] Added deterministic boundary `risk` and `reasons`, ranking third-party public/production spread above first-party or unresolved API exposure while making standard-library, test-only, and package-internal local spread informational.
- [x] Added content- and grammar-addressed resolved-navigation caching under `.grepple/cache/boundaries/` with atomic writes and corrupt-entry fallback.
- [x] Kept cache state out of report output so cold and warm reports remain byte-identical.
- [x] Classified spread as package-internal, cross-package, cross-layer, or public API and containment as approved, escaped, or unknown from generated ownership facts plus optional repository policy.
- [x] Added validated `grepple-boundary-policy-v1` layers, containment, facades, and intentional path classifications through `.grepple/boundary-policy.json` or `--policy`.
- [x] Added generic resolved-call facade-bypass findings without language or framework policy in the shared analyzer.
- [x] Extended owner surfaces and type spread to typed fields/properties with adapter-owned visibility and honest unambiguous ownership requirements.
- [x] Distinguished public API, private signature, field representation, body-local, and unknown type surfaces.
- [x] Down-ranked standard-library, test-only, utility-hub, test-framework, declarative-configuration, lifecycle-cleanup, and adapter-protocol leads without removing evidence.
- [x] Added reviewed precision fixtures for utility hubs, owner cohesion, repeated protocols, parallel abstractions, transitive public exposure, and policy-backed misplaced-function signals.

## Capability reporting and language support

- [x] Added deterministic human, JSON, and generated Markdown capability views covering extensions, segments, outlines, navigation, focused structure/flow, and GritQL.
- [x] Added parity tests proving advertised combinations work and unsupported combinations fail explicitly.
- [x] Added JavaScript/JSX, Python, Java, Kotlin, and C# focused architecture adapters on the shared parser/navigation infrastructure.
- [x] Added native GritQL support for every Tree-sitter-backed language in the capability registry.

## Reliability and performance

- [x] Added malformed and partially written source fuzz seeds.
- [x] Added bounded fuzz targets for navigation extraction and both Mermaid parsers.
- [x] Made generated diagrams round-trip through their checkers and byte-deterministic in current tests.
- [x] Added parse, navigation-index, cache-format, focused extraction, and directory architecture benchmarks; retired package/workspace bundle benchmarks with those features.
- [x] Compared JSON, gob, protobuf variants, packed layouts, and experimental native tree serialization for cache design.
- [x] Added reviewed runtime/allocation budgets as benchmark gates rather than host-sensitive unit assertions.
- [x] Kept `go test -race ./...`, generated parser metadata checks, and deterministic architecture tests green through the completed refactors.
- [x] Added reproducible version, revision, source time, toolchain, and platform output through `--version`.
