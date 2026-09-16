# Solved improvements

This file records completed Grepple roadmap work that no longer belongs in `next.md`. It is a capability history, not a list of current priorities.

## Agent workflows and guidance

- [x] Proved the workspace summary → package summary → exact `--at` → bounded `--related` workflow for unfamiliar repositories.
- [x] Added short intent-triggered skills for local retrieval, architecture lookup, change impact, boundary review, structural audits, architecture diagrams, and remote inspection.
- [x] Made each skill state what Grepple evidence can and cannot prove, with explicit handling for candidates, bounded output, source universes, and syntax-only analysis.
- [x] Added repeatable agent workflow benchmarks for breadth summaries, outlines, structural lookup, exact ranges, related navigation, impact graphs, enclosing scopes, and edit locations.
- [x] Added answer-gated task benchmarks for bounded directory orientation, direct architecture resolution, and exact relation explanation; each task completes in one modeled retrieval call.
- [x] Added `grepple ask` for bounded, cost-efficient research through Fantasy, with provider-neutral authentication, Codex device login and refresh, direct typed read-only tools, confined local reads, and a user-owned default research model in `~/.grepple/grepple.json`.
- [x] Added a per-invocation ask research cache identified by canonical source root, repository configuration digest, active source flags, server, tool, and normalized typed input. Successful identical calls preserve evidence bytes, concurrent duplicates share one execution, failures retry, canceled waiters do not cancel shared work, and response metadata plus semantic JSONL events disclose cache status.
- [x] Expanded the ask provider registry with GitHub Copilot device authorization, Anthropic API-key and subscription setup-token modes, OpenAI API keys, and AWS Bedrock default-chain authentication. Static secrets use hidden prompts and the protected provider store; Copilot validates the subscription and exchanges its GitHub token for a short-lived model token; Bedrock stores no AWS secret. One `<provider>/<model>` `ask.model` value selects both dimensions without cross-provider model leakage, and `ask.logs` controls logging plus automatic retention cleanup.
- [x] Added scope-keyed lazy local research universes for ask. `navigate_code`, `query_graph`, and `inspect_architecture` reuse caller-owned parser documents, outlines, and one resolved navigation analysis; equivalent path orderings share state, sessions close documents once, and cold/reused outputs are byte-identical. A 30-file three-tool benchmark reduced runtime from 12.93 ms to 4.59 ms and allocations from 2.47 MB/63,628 to 1.01 MB/23,497.

## CLI discoverability

- [x] Added top-level help that lists search, GritQL, graph, boundaries, extraction, language capabilities, rules, remote repository/authentication commands, and version before the default search options.
- [x] Added real `grepple help` and command help routing plus an explicit `grepple search` mode so command-name patterns do not collide with dispatch.
- [x] Added successful recursive help for every extract and graph mode, including explicit output-mode and root-selector exclusivity contracts.
- [x] Added `grepple examples [TASK]` with concise copyable workflows for orientation, exact retrieval, editing, caller impact, boundary review, GritQL audits, focused diagrams, and canonical checks.
- [x] Clarified directly in search help that `-l`/`--files` lists paths rather than matching content, identified `--files-with-matches` as the `grep -l` equivalent, and put the content-matching mode first in the copyable retrieval workflow.

## Repository scope and output delivery

- [x] Extended the nearest ancestor root `grepple.json` with validated repository-relative `ignore.paths` and `output.spillThresholdBytes` while preserving existing `server` files.
- [x] Kept authentication user-owned: repository configuration cannot provide or override access tokens, refresh tokens, expiries, or user identity.
- [x] Applied one ordered `**`/negation-capable ignore matcher to search, graph, boundaries, GritQL, and focused extraction; recursive discovery honors ignores while explicitly named files bypass them.
- [x] Excluded `.grepple/cache/` and `.grepple/output/` from recursive source discovery and Git.
- [x] Added default 64 KiB output spilling to content-addressed mode-0600 artifacts with valid human/JSON descriptors, original schema/source metadata, exact `--no-spill` reruns, threshold overrides, and an explicit global `--artifact-dir PATH` destination.
- [x] Added answer-gated artifact workflow benchmarks covering context bytes, retrieval turns, artifact reads, and correctness; one bounded artifact read reduced a synthetic 1 MiB task from 1,048,676 to 902 context bytes while recovering the same answer.
- [x] Added `grepple artifacts clean` for explicit deterministic artifact cleanup.
- [x] Replaced process-exiting library paths with propagated command exit statuses so output delivery is finalized before grep-style exit code 1.
- [x] Added `grepple sources explain` with deterministic config path/digest, selection decisions, classification totals, exclusion reasons, explicit bypasses, and omitted infrastructure subtrees.
- [x] Added global `--no-repo-config`, `--no-config-ignore`, and `--production-only` source controls without disabling user authentication state.
- [x] Classified recursive sources as production, test, fixture, generated, or vendor; exposed directory totals and retained explicit-file precedence with a bypass notice.
- [x] Derived graph-level Go repository roots from selected modules and unambiguous local `go.mod`/`go.work` replacements, independently of optional declaration metadata.
- [x] Covered nested modules, workspace replacements, conflicting replacements, ambiguous unqualified imports, local replacement call resolution, and cold/warm plus focused/full identity parity.
- [x] Included module/workspace files in boundary cache digests and upgraded the cache schema so repository identity changes cannot reuse stale origin results.

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
- [x] Added complete `grepple-navigation-graph-v2` JSON and bounded compact graph projections, including adapter-evidenced entrypoints and routes.
- [x] Added deterministic discovered, selected, parsed, skipped, failed, recovered, and truncated source accounting to graphs, focused graph queries, graph diffs, and boundary reports.
- [x] Added graph-wide and per-language/confidence resolution measurements, including resolved, ambiguous, unresolved, singleton-candidate counts, and ambiguity frequency in complete JSON plus compact aggregate headers.
- [x] Added parser-owned content- and grammar-addressed per-file navigation facts shared by graph output/queries, `--related`, boundaries, and extraction, with atomic corruption-tolerant storage, path instantiation, explicit bypass, cold/warm parity tests, and a repeatable benchmark.
- [x] Enriched path-neutral cached Go facts with owning module/package identity after loading so host-qualified same-module imports classify as first-party instead of critical third-party leakage.
- [x] Added callers, callees, dependencies, dependents, and bidirectional impact queries with depth bounds, cycle safety, candidate preservation, and exact location/symbol/package/module/path roots.
- [x] Added traversal-free `graph resolve --symbol NAME` previews with deterministic exact/terminal matches, full stable IDs, exact `--at` alternatives, filters, and copyable focused graph commands in bounded compact and complete JSON output.
- [x] Added language, confidence, and visibility filtering before root selection and traversal.
- [x] Added `grepple-navigation-diff-v2`, ignoring line-only movement while reporting semantic declaration, call-edge, entrypoint, and route changes.
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
- [x] Added adapter-owned import facts and local target paths to the shared navigation graph for Go and JavaScript/TypeScript, with explicit language capability reporting and cache-safe path instantiation.
- [x] Added independently labeled import-only and imported-type directory relations with production/test/fixture/generated/vendor evidence totals and resolved/ambiguous/unresolved/unsupported coverage.
- [x] Kept compact directory relation output bounded to displayed directory depth while `architecture why` preserves exact uncollapsed evidence.
- [x] Added `parser.OutlineFromDocument` and document-backed graph construction so directory architecture parses each selected source once for both outlines and navigation.
- [x] Added adapter-evidenced Go process entrypoints and source-linked `net/http.Handle`/`HandleFunc` routes to shared graph, directory architecture, focused queries, graph filtering, and semantic diffs without inferring unsupported framework semantics.
- [x] Added a selected source-file inventory and `architecture compare` diagnostics that normalize paths and collection ordering before raw-byte checks, then report the first changed file, declaration, route, relation, or directory with exact before/after evidence.
- [x] Removed Go-specific package/workspace commands, public capability fields, generators, checkers, hook paths, tests, and committed `.grepple` artifacts without a compatibility period.

## Parser and language architecture

- [x] Moved application-source parsing behind `parser.Document`, callback-scoped `DocumentView`, and shared traversal/range APIs.
- [x] Documented the ownership, locking, invalidation, retention, recovery, and performance contracts for `Document`, `Node`, `DocumentView`, `ViewNode`, and immutable `SyntaxNode` snapshots.
- [x] Benchmarked document-tied, callback-scoped, and immutable-snapshot syntax traversal; retained all three because coherent lock scope, retainable borrowed handles, and post-close ownership are distinct contracts rather than aliases.
- [x] Removed extraction and hook parser construction, grammar selection, and duplicate application-source parser lifecycles.
- [x] Added a parser-owned language capability registry with canonical IDs, extensions, grammar ABI/fingerprints, navigation support, and generated grammar cardinality/subtype metadata.
- [x] Added iterative bounded syntax traversal and immutable subtree snapshot reuse within document views.
- [x] Moved callable, declaration, call, container, import, binding, return, visibility, receiver, member, and field syntax policy behind `languageAdapter.Navigation()`.
- [x] Reduced `parser/tree_sitter.go` to syntax-backend ownership by moving segment match/punctuation helpers to `segments.go` and adapter naming policy to `language.go`.
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
- [x] Added Google Release Please automation starting at `v0.0.1`, native-CGO GoReleaser builds and smoke tests for Linux amd64/arm64, macOS amd64/arm64, and Windows amd64, SHA-256 release assets, and plan-aware build-provenance attestations for public repositories.
- [x] Added weekly grouped Dependabot updates for the root Go module, hooks module, and GitHub Actions.
- [x] Added callable-declaration preflight to exact local navigation: package, import, blank, and other file-scope locations now return bounded source without constructing the repository-wide related graph; the typed ask tool adds an explicit correction while callable locations retain related traversal. A previously observed 14–27 second invalid navigation path completed in 0.06 seconds after the change.
