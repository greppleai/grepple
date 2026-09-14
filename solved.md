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

## Search and retrieval

- [x] Added `--count-summary` for complete matched-file and matching-line breadth independent of output paging.
- [x] Kept grep-compatible per-file `--count` and grouped `--count-by-repo` output.
- [x] Added safe compatibility aliases: `-E` selects JavaScript regex and `-r` is a recursive-search no-op.
- [x] Bounded human-readable output at 16,384 bytes while keeping JSON valid and byte-uncapped.
- [x] Reported omitted segments and broad-output truncation with narrowing guidance.
- [x] Added parser-backed construct ranges to `--line-only` and opt-in enclosing ranges without changing anchor rows.
- [x] Preserved exact `HASH│LINE│content` output through a user-owned, versioned anchor-provider protocol.
- [x] Added provider digest checks, timeout/output bounds, strict response validation, default enablement, and `--no-anchors` fallback.
- [x] Streamed validated focused extraction to stdout when no output path is supplied.
- [x] Classified default structural-search results as structured, recovered, plain, unsupported, or failed; incomplete human output is flagged and complete JSON carries per-file plus aggregate status.

## Shared navigation graph

- [x] Added one normalized `parser.NavigationGraph` with stable declarations, calls, candidates, source ranges, confidence, visibility, package/module/container context, imports, receivers, return types, type usages, and member accesses.
- [x] Made `--at`, `--related`, focused flow generation/validation, JSON, compact output, graph queries, semantic diffs, and parity tests consume the shared projection.
- [x] Preserved `Symbol.Calls` and `Symbol.CallOrder` as compatibility projections instead of independent call collectors.
- [x] Added deterministic ambiguity ranking and actionable `--at PATH:LINE` candidate suggestions.
- [x] Added exact, import-resolved, context-resolved, unique-terminal, and candidate confidence classes.
- [x] Resolved explicit Go package imports and TypeScript named/namespace imports before terminal fallback.
- [x] Added direct receiver/parameter context, source-ordered lexical bindings, local/imported return inference, and same-file typed member chains.
- [x] Reported omitted callers and callees in human and JSON related output.
- [x] Made every human omitted-edge notice provide a copyable focused `graph callers|callees|impact --at PATH:LINE --depth 2 --json .` continuation.
- [x] Added complete `grepple-navigation-graph-v1` JSON and bounded compact graph projections.
- [x] Added deterministic discovered, selected, parsed, skipped, failed, recovered, and truncated source accounting to graphs, focused graph queries, graph diffs, and boundary reports.
- [x] Added parser-owned content- and grammar-addressed per-file navigation facts shared by graph output/queries, `--related`, boundaries, and extraction, with atomic corruption-tolerant storage, path instantiation, explicit bypass, cold/warm parity tests, and a repeatable benchmark.
- [x] Added callers, callees, dependencies, dependents, and bidirectional impact queries with depth bounds, cycle safety, candidate preservation, and exact location/symbol/package/module/path roots.
- [x] Added language, confidence, and visibility filtering before root selection and traversal.
- [x] Added `grepple-navigation-diff-v1`, ignoring line-only movement while reporting semantic declaration and call-edge changes.
- [x] Added end-to-end parity coverage across JSON, compact output, focused Mermaid, canonical Mermaid, and `--related`.

## Architecture extraction and validation

- [x] Replaced hook-local Mermaid extraction/checking with the shared root `extract` implementation.
- [x] Added source-linked, deterministic structure and flow generation with bounded depth/node counts.
- [x] Added explicit, checker-valid truncation markers to bounded diagrams.
- [x] Added structure, flow, package, and workspace checking.
- [x] Added canonical Go package/workspace overview, structure, and manifest bundles.
- [x] Added bounded package/workspace Markdown summaries; dogfooding reduced package orientation from about 27.5 KB to 2.1 KB and workspace orientation from about 4.3 KB to 2.4 KB.
- [x] Added compact architecture presentation that omits Mermaid validation metadata while retaining canonical bundles.
- [x] Added exact declaration/member ranges and deterministic relation evidence to generated artifacts.
- [x] Added focused structure/flow support for Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, and C#.
- [x] Added semantic graph diffing for code review and CI architecture impact.

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

## Boundary analysis

- [x] Added `grepple boundaries [PATH...]` over the shared navigation graph.
- [x] Added file-owned workflow signals for external consumer breadth, callable surface, callable co-usage, ordered sequences, and member-read/write plus call combinations.
- [x] Excluded owner-local calls and required repeated evidence across at least two external files.
- [x] Added import-qualified and unambiguously owned project type spread with parameter/result/receiver/local roles, public signature exposure, and production/test reach.
- [x] Classified type origin independently as local, first-party, standard-library, third-party, or unresolved while retaining imported identity and the compatibility `external` signal.
- [x] Added deterministic boundary `risk` and `reasons`, ranking third-party public/production spread above first-party or unresolved API exposure while making standard-library, test-only, and package-internal local spread informational.
- [x] Added content- and grammar-addressed resolved-navigation caching under `.grepple/cache/boundaries/` with atomic writes and corrupt-entry fallback.
- [x] Kept cache state out of report output so cold and warm reports remain byte-identical.

## Capability reporting and language support

- [x] Added deterministic human, JSON, and generated Markdown capability views covering extensions, segments, outlines, navigation, focused structure/flow, GritQL, and canonical bundles.
- [x] Added parity tests proving advertised combinations work and unsupported combinations fail explicitly.
- [x] Added JavaScript/JSX, Python, Java, Kotlin, and C# focused architecture adapters on the shared parser/navigation infrastructure.
- [x] Added native GritQL support for every Tree-sitter-backed language in the capability registry.

## Reliability and performance

- [x] Added malformed and partially written source fuzz seeds.
- [x] Added bounded fuzz targets for navigation extraction and both Mermaid parsers.
- [x] Made generated diagrams round-trip through their checkers and byte-deterministic in current tests.
- [x] Added parse, navigation-index, cache-format, focused extraction, package bundle, and workspace bundle benchmarks.
- [x] Compared JSON, gob, protobuf variants, packed layouts, and experimental native tree serialization for cache design.
- [x] Added reviewed runtime/allocation budgets as benchmark gates rather than host-sensitive unit assertions.
- [x] Kept `go test -race ./...`, canonical schema checks, and package/workspace drift checks green through the completed refactors.
- [x] Added reproducible version, revision, source time, toolchain, and platform output through `--version`.
