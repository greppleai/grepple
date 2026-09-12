# Next improvements

Grepple should stabilize for one or two iterations before adding more languages. The recent work introduced a large extraction, navigation, CLI, schema, and hook surface.

## Recommended next milestone

> Continue the agentic-coding feedback work before further language expansion: use the new workflow benchmark to design a smaller architecture summary and evidence-based agent output limits. C# remains the next class-model parity target after those ergonomics improvements.

## Feature set versus feature completeness

Treat these as separate planning dimensions:

- **Feature set** determines which workflows Grepple makes possible. New graph projections and operations can unlock entirely new agent, developer, and CI use cases.
- **Feature completeness** determines how precisely an existing workflow handles language and type-system edge cases. Import, receiver, lexical-scope, and return-type resolution belong here.
- Completeness still needs a minimum reliability floor: deterministic candidates, bounded output, no unsafe binding leakage, and actionable ambiguity must remain release gates.
- After that floor is met, prefer a new usable capability over indefinitely refining syntax-based resolution. Return to completeness work when dogfooding demonstrates that a specific ambiguity blocks a valuable workflow.

## Findings from dogfooding

### What is already strong

- The workspace → package overview → `--at` → `--related` workflow is a fast and effective way to enter an unfamiliar codebase.
- Structural search is highly token-efficient because it preserves the enclosing declaration while collapsing unrelated code. In the latest dogfood round, outlining a 581-line/19.5 KB JVM extractor returned 50 lines/1.9 KB, while a structural declaration lookup returned all relevant declarations in one 1.8 KB response.
- Exact declaration and member ranges make generated diagrams useful navigation maps rather than passive documentation. Focused extraction already streams validated Mermaid to stdout when `--output` is omitted.
- Focused flow is concise and readable for small call chains across the production Go, ECMAScript, Python, Java, and Kotlin adapters.
- Canonical package and workspace checks make architecture documentation trustworthy enough to use as an index.
- Deterministic ordering, bounded traversal, explicit confidence, and round-trip validation are strong foundations for agent use.
- `--count-summary` now reports complete matched-file and matching-line breadth independently of paging, while `--count` retains grep-compatible per-file page output.

### Remaining friction

- External LLM dogfooding confirmed the Grepple locate/orient → Edit loop, while also exposing grep-flag muscle memory, edit-target lines omitted by segment limits, and broad output reaching the agent harness limit.
- The shared `parser.NavigationGraph` is now the sole source of generic call edges for flow selection, rendering, validation, `--at`, and `--related`; the next graph issues are focused querying and richer resolution rather than duplicate discovery.
- `--related` can exhaust its edge budget without reporting how many callers or callees were omitted. Bounded output must never look complete during impact analysis.
- Broad structural output is safely capped at 40960 bytes, but that still costs roughly 10000 tokens. Untrained agents need a cheaper default/profile and clearer early narrowing.
- Generated package overviews can still be too large for focused questions: the extract package overview reached 526 lines/27 KB, while a file outline answered the immediate JVM question in 50 lines/1.9 KB.
- Larger diagrams are dominated by validation metadata when viewed as plain text.
- Agent edit anchors are provider-specific, not generic line hashes. Grepple delegates through a user-owned, versioned batch provider and keeps editing-harness adapters out of the shipped repository.
- Explicit Go and TypeScript imports, direct parameters/receivers, source-ordered lexical bindings, local and imported return signatures, and same-file typed field chains now resolve before terminal-name fallback. Cross-file field chains, embedded/promoted methods, re-exports, and default imports still need richer propagation.
- Architecture extraction remains narrower than parser-backed segments, outlines, navigation, and native GritQL. Python, Java, and Kotlin now join Go and ECMAScript with production focused structure/flow. C# is the remaining high-value object-model gap.
- Cross-language fixtures prove the baseline but do not yet cover enough malformed, nested, generic, decorated, or multiline syntax.
- Canonical mismatch errors identify the artifact but should eventually report the first semantic difference.

## Highest priority

### 1. Unify navigation and flow graphs

- [x] Add a normalized `parser.NavigationGraph` with source identity and parse-tree reuse.
- [x] Migrate `--related` and `--at` entry resolution to the normalized graph.
- [x] Record that graph in `extract.Analysis` without a second parse.
- [x] Rebase Go and TypeScript flow traversal fully on the shared graph while retaining language-specific enrichment for imports, receivers, packages, and modules.
- [x] Add parity tests asserting that focused Go and TypeScript flow edges exist in the shared graph.
- [x] Keep dependencies directed toward `api` and `parser`; `search` does not depend on `extract`.

### 2. Fix observed usefulness gaps

- [x] Include TypeScript method parameter and result references in focused structure traversal. For example, `Store.load(): Item` now includes `Item` and a dependency edge.
- [x] Generate deterministic, validated structure and flow truncation with an explicit `grepple:truncated` marker when `--max-nodes` is reached.
- [x] Use safe declaration-kind, exact-qualifier, and same-file context to reduce terminal-name ambiguity; continue with package, receiver, module, and import-aware resolution.
- [x] Accept safe grep compatibility aliases: `-E` explicitly selects the default JavaScript-regex mode and `-r` is a no-op because directory search is already recursive.
- [x] Bound human-readable output below common agent limits with actionable narrowing guidance while keeping JSON valid and uncapped.
- [x] Report matching lines omitted by `--max-segments` and direct edit-oriented searches to `--line-only`, which emits configured anchors or supplies exact lines for Read fallback.
- [x] Report related-edge truncation explicitly, including omitted caller/callee counts in human and JSON output plus complete-graph guidance. Tests prove bounded previews cannot be mistaken for complete impact results.
- [x] Add `--count-summary` for complete aggregate matched-file and matching-line totals independent of `--skip`, `--limit`, and `--max-files`; retain `--count-by-repo` for grouped compatibility output.
- [ ] Add an explicit agent-oriented output profile, or evaluate a lower default human-output budget, so broad accidental queries stop well below common context limits without affecting complete JSON.
- [x] Stream validated `extract structure|flow` projections to stdout when `--output` is omitted.
- [ ] Add a smaller package/workspace summary projection for orientation when generated overviews are still too large.
- [ ] Consider a compact presentation mode that hides validation metadata while retaining it in generated artifacts.

### 3. Harden source handling

- [ ] Add larger golden fixtures for every supported language. Existing cross-language navigation fixtures cover the minimum set but need broader syntax.
- [x] Seed fuzz coverage with malformed and partially written source.
- [ ] Add language-specific cases for nested declarations, decorators or annotations, generics, and multiline signatures.
- [ ] Expand CRLF, symlink, build-constraint, ambiguous-extension, and cross-platform path tests.
- [x] Add bounded fuzz targets for navigation extraction and both Mermaid parsers.

## Immediate execution order

### A. Finish shared-graph flow traversal

1. [x] Add stable declaration, caller, call, and target identities to `parser.NavigationGraph`.
2. [x] Enrich graph declarations and calls in `extract` with package/module identity, imports, receiver/container information, resolved names, semantic targets, and confidence.
3. [x] Make Go and TypeScript flow selection, rendering, and validation consume enriched graph edges instead of independently collected `Symbol.CallOrder`.
4. [x] Remove the independent Go and TypeScript call collectors; preserve `Symbol.Calls` and `Symbol.CallOrder` compatibility as projections derived from the shared graph.
5. [x] Cover exact edge parity, selected-node order, and truncation boundaries for Go and TypeScript.

Acceptance criteria met for generic call discovery: one parser-owned graph supplies `--related`, focused flow generation, and flow validation; language adapters only enrich and resolve graph facts.

### B. Improve resolution confidence

1. [x] Resolve explicit Go package imports and TypeScript named/namespace imports before terminal-name fallback.
2. [x] Record direct parameter, Go method-receiver, and TypeScript `this` containing-type context for method calls.
   - [x] Propagate source-ordered lexical bindings from typed declarations and constructor/composite literals without leaking future, sibling, branch-local, or nested bindings.
   - [x] Infer lexical binding types from unambiguous local and imported Go and TypeScript return signatures.
   - Completeness backlog after the graph feature-set work: propagate field types across files, embedded/promoted methods, re-exports, and default imports.
3. [x] Distinguish exact, import-resolved, context-resolved, unique-terminal, and candidate confidence.
4. [x] Rank unresolved candidates deterministically and suggest `--at PATH:LINE` locations.
5. [x] Add duplicate same-name package/module function and method fixtures covering Go aliases and TypeScript named imports.
6. Add ambiguity fixtures for interfaces, overloads, inheritance, re-exports, default imports, and TS/TSX path aliases.

### C. Expand the usable graph feature set

These are the next capability milestones. The command names below are potential interfaces, not commitments; implementation should choose the smallest coherent CLI surface backed by the shared graph.

#### C1. Normalized JSON graph output — complete

[x] `grepple graph --json [PATH...]` emits the versioned `grepple-navigation-graph-v1` projection: declarations with stable IDs and source line ranges; calls with caller IDs, resolved target IDs or deterministic candidate target IDs, import/receiver context, confidence, and enclosing ranges; and explicit deterministic `--max-files` truncation. It builds through the same search navigation resolver used by `--related` and preserves parser graph identities.

Potential examples:

```bash
grepple graph --json .
grepple graph --json ./internal/cli --max-files 200
```

Use case: an agent, editor extension, or CI job can consume exact nodes and edges, join them by stable ID, filter candidates by confidence, and retain complete machine-readable output independently of presentation.

#### C2. Compact agent-facing architecture output — complete

[x] `grepple graph --compact [PATH...]` emits declarations and directed calls from the same normalized graph, retaining stable ID prefixes, source locations, confidence, candidate identities, and file-truncation warnings while omitting JSON and Mermaid validation detail. Human output is bounded by the shared 40960-byte default; JSON remains complete and uncapped.

Potential examples:

```bash
grepple graph --compact ./internal/cli
grepple graph --compact ./search --max-output-bytes 20000
```

Use case: an agent can orient itself in a package or focused call chain within a small token budget, then jump to exact declarations with `--at` without reading a large generated bundle.

#### C3. Graph queries

- [x] Add deterministic `graph callers`, `callees`, `dependencies`, `dependents`, and bidirectional `impact` traversal with bounded depth, cycle protection, candidate-edge preservation, complete JSON, and bounded compact output. Exact symbol/location selectors choose one root; package/module/root-path selectors choose a scope.
- [x] Add repeatable canonical language and confidence filters before root selection and traversal; preserve normalized filter metadata in JSON and compact output.
- [ ] Add exportedness filtering after declaration adapters expose a reliable cross-language exported/public fact. Add edge-kind filtering when the normalized graph contains more than call edges. Queries and `--related` must expose omitted-edge counts whenever bounds make the result incomplete.

Potential examples:

```bash
grepple graph callers --symbol 'Service.Save' --language go --confidence exact --depth 2 --compact .
grepple graph callees --at internal/cli/local.go:120 --depth 2 --json .
grepple graph dependencies --root-path internal/cli --depth 1 --compact .
grepple graph impact --at internal/cli/local.go:120 --depth 3 --compact
```

Use case: before editing a declaration, an agent or developer can identify likely consumers, affected packages, and relevant tests without manually traversing repeated `--related` responses.

#### C4. Architecture and graph diffing

Compare two normalized graphs or canonical snapshots and report added, removed, moved, and changed declarations and edges. Prefer semantic identity over line-based Mermaid diffs, with bounded human output and complete JSON.

Potential examples:

```bash
grepple graph diff --base main --head HEAD --compact
grepple graph diff .grepple-before/ .grepple/ --json
```

Use case: code review and CI can explain architectural impact—such as a new package dependency, removed route, or changed caller edge—rather than only reporting that a generated artifact differs.

#### C5. Shared-projection guarantees

1. Derive compact text, JSON, graph queries, diffs, and canonical Mermaid from the same normalized graph.
2. Preserve stable node identity, source ranges, edge confidence, and truncation semantics in every applicable projection.
3. Add parity tests proving that compact and JSON edges agree with canonical Mermaid and `--related`.

### C6. Remove the redundant Read call for edits

1. [x] Add user-owned `~/.grepple/settings.json` anchor-provider configuration; never execute repository-controlled provider commands.
2. [x] Add a versioned whole-file batch protocol with digest checks, timeout/output limits, strict response validation, and direct process execution without a shell.
3. [x] Emit unambiguous `HASH│LINE│content` rows from local structural and `--line-only` output while retaining synthetic summary markers.
4. [x] Verify locally that a user-owned provider can produce anchors accepted by the configured editing harness without shipping harness-specific adapter code.
5. [x] Allow user settings to enable anchors by default for compatible output, with `--no-anchors` as a per-command escape hatch and automatic fallback for incompatible modes.
6. [ ] Add `grepple anchors doctor` (or equivalent) to report provider identity/protocol, test a temporary file, and diagnose stale or missing configuration.
7. [ ] Make the Pi/user integration offer an explicit provider-setup path so a fresh agent environment can enable edit-ready output without repository-owned commands or silent configuration changes.
8. [ ] Evaluate anchors for related previews and context output only if dogfooding shows they save additional tool calls; keep JSON provider-neutral.


### D. Expand hardening and performance coverage

1. Grow each language fixture with malformed, nested, generic, decorated/annotated, and multiline declarations.
2. Add CRLF, symlink, build-tag, ambiguous-extension, and Windows-path cases.
3. Run fuzz targets for longer periods in scheduled CI and retain minimized regressions as seeds.
4. Benchmark parse, navigation-index construction, focused extraction, package bundles, and workspace bundles.
   - [x] Add a reproducible warm-page-cache comparison of Tree-sitter parsing, parse-plus-navigation extraction, normalized graph caches, full read-only CST projections, and experimental native `TSTree` serialization. Compare JSON, gob, manual protobuf wire code, standard `protoc-gen-go`, and `vtprotobuf` for message-oriented and string-interned packed layouts; report corpus/cache sizes, throughput, allocations, source-file count, and native serialize/deserialize cost.
   - Next: add focused extraction and package/workspace bundle benchmarks.
5. Add performance budgets that detect repeated parsing and significant allocation/runtime regressions.
6. [x] Add a repeatable agentic-coding benchmark suite with fixed breadth-summary, outline, structural lookup, line-plus-`--at`, related-navigation, impact-graph, and edit-location tasks. It verifies answer fragments and reports tool-call count, returned bytes, approximate tokens, elapsed time, and allocations. The initial baseline confirms that structural lookup trades 16 additional bytes for one fewer retrieval call than line-only plus `--at`; methodology and measurements live in `docs/agent-workflow-benchmarks.md`.

### E. Improve architecture drift diagnostics

1. Compare normalized manifests before byte-level artifact comparison.
2. Report the first changed declaration, member, route, relation, package, or module.
3. Keep the final byte comparison as the canonical determinism check.

### F. Consolidate parser and source infrastructure

Reduce repeated parsing and language knowledge across extraction, navigation, text search, and GritQL without moving query semantics into `parser`. The intended result is one source document, one language registry, and reusable projections—not one oversized package.

1. [x] Migrate `extract` away from direct Tree-sitter parser construction.
   - [x] Make Go, TypeScript, and TSX extraction consume `parser.Document` and callback-scoped `parser.ViewNode` values.
   - [x] Remove extraction- and hook-local grammar selection and parser lifecycle; application-source parsing now flows through `parser.ParseDocument`.
   - [x] Preserve exact source ranges, malformed-source behavior, and generated artifact semantics. `parser.NavigationGraphFromDocument` now lets extraction derive navigation from the same parsed document without reparsing.
2. [x] Expose a read-only parser language capability registry.
   - [x] Make parser-backed language IDs, extensions, ABI/fingerprint identity, and navigation support centrally discoverable; extraction delegates path classification and extension metadata to that registry.
   - [x] Generate exact grammar source fingerprints plus field and unfielded-child cardinality from every pinned grammar. GritQL now consumes parser-owned Go repeated-position metadata instead of handwritten tables, and `make schema-check` rejects stale generated metadata.
   - [x] Keep Grit-specific snippet wrappers and metavariable placeholder roles in `gritql`; they are query semantics rather than general parser facts.
3. [x] Add efficient shared syntax traversal primitives.
   - [x] Add parser-owned iterative named-node walking and use parser-owned child, field, diagnostic, text, and range access throughout extraction and hooks.
   - [x] Add bounded/depth-aware walking and a callback-scoped `DocumentView`. Extraction, hooks, and GritQL evaluation now hold one read lock per document instead of one per node operation.
   - [x] Cache immutable subtree snapshots within a document view, so matching overlapping candidates reuses descendants rather than rebuilding every subtree. A repository-wide `exec.Command($args)` dogfood scan fell from roughly 8.8 seconds to 1.5 seconds elapsed on the same checkout.
   - [x] Keep feature-specific filtering such as GritQL trivia normalization and navigation declaration rules outside generic parser helpers.
4. [x] Introduce GritQL language adapters over parser capabilities.
   - [x] Keep the compiler, query algebra, bindings, matching, constraints, transactional findings, and diagnostics in `gritql`, while isolating target parsing and root categories behind adapters.
   - [x] Use one `gritql-v1` compatibility contract for Go, TypeScript, and TSX, with focused expression, type, statement, declaration, sequence, JSX, metavariable, and repeated-list matching.
   - [x] Generate grammar subtype relationships alongside cardinality, so adapters do not grow handwritten expression/type/statement kind tables.
   - Follow-up completeness: expand TypeScript/TSX grammar-local placeholder roles beyond the implemented identifier-like, import-source, and whole-declaration positions when a target grammar position cannot be represented safely by those forms.
   - [x] Add dedicated TypeScript/TSX conformance fixtures covering ambiguous expression/type syntax, imports, sequences, JSX, query algebra, and binding equality.
   - [x] Group mixed-language saved-rule batches by target language while reading and parsing each eligible source only once for the rules matching that source language.
   - [x] Reject unsupported languages and compatibility mismatches explicitly; no target uses a fallback parser.
5. [x] Unify source discovery and acquisition where text and structural search overlap.
   - [x] Reuse `search.ListFilePathsContext` for cancellable repository-relative discovery, ignore handling, globs, and deterministic ordering, then perform language filtering, bounded acquisition, binary detection, cancellation, and evaluation in the reusable Grit scanner.
   - [x] Evaluate mandatory anchors inside the bounded Grit acquisition pass, so local structural search reads every scoped source at most once and parses only anchor-selected files. Mixed-rule batches prefilter independently per program before their shared parse.
   - [x] Keep evaluation orchestration reusable by local CLI and backend callers without making `gritql` depend on `search`; preacquired callers may still provide `ScanCandidate.Content` without bypassing scanner checks.
6. Unify feature support across the existing language set.
   - [x] Add one deterministic, machine-readable capability view (and a concise CLI presentation) for each parser-owned language ID: extensions, segments, outlines, navigation, focused structure/flow, native GritQL, and canonical bundle support. `grepple languages`, `--json`, and generated Markdown now derive from parser, extraction, and GritQL registrations. Feature implementations remain owned by their packages; language identity and grammar metadata remain owned by `parser`.
   - [x] Add JavaScript/JSX to the unified `gritql-v1` contract by reusing the TypeScript-family adapter where grammar behavior agrees, with dedicated conformance fixtures for declarations, expressions, imports, JSX, sequences, containment, bindings, malformed source, and repeated positions.
   - [x] Add JavaScript/JSX focused structure and flow extraction through the shared navigation graph and ECMAScript analysis helpers. Canonical package/workspace bundles remain Go-specific.
   - [x] Add cross-feature parity tests proving advertised capabilities are implemented and that unsupported combinations fail explicitly rather than silently falling back. Registration parity, generated documentation drift, JavaScript/JSX CLI/conformance, malformed-source, extraction round-trip, and unsupported-language tests cover the matrix.
   - [x] Add Python to `gritql-v1` with expression, pattern, statement, declaration, module, dotted-import, repeated-list, malformed-source, mixed-batch, CLI, and conformance coverage.
   - [x] Add C, C++, C#, Java, Kotlin, Rust, and Shell to `gritql-v1` through bounded grammar-local snippet adapters. Every parser-backed Tree-sitter language now has native GritQL, scanner, malformed-source, binding-equality, and conformance coverage.
   - [x] Add Python focused structure and flow extraction using `parser.Document` and the shared navigation graph, with classes, inheritance, annotated and unannotated attributes, decorators, `.pyi` stubs, bounded traversal, validation, and deterministic module metadata.
   - [x] Add Java focused structure/flow with classes, interfaces, records, enums, inheritance, implementations, fields, constructors, methods, conservative cross-file targets, and shared navigation-graph flows.
   - [x] Add Kotlin focused structure/flow with classes, interfaces, objects, data-class constructor properties, delegation-based inheritance, properties, functions, conservative cross-file targets, and shared navigation-graph flows.
   - [ ] Add C# focused structure/flow next. Keep C/C++, Rust, and Shell unsupported until their non-class architecture projections have an explicit useful contract.
Recommended implementation order from here: a smaller architecture summary and evidence-based agent output limits; then exported/public declaration facts and filters, C# focused structure/flow, semantic graph diffing, and useful non-class projections for Rust or C/C++. Address grammar-local placeholder-role gaps when conformance exposes a valuable blocked workflow.

Acceptance criteria:

- No feature outside `parser` constructs a parser for supported application source directly; parsing the GritQL query language itself remains an intentional exception.
- Every advertised language/feature combination has parity coverage, and unsupported combinations fail explicitly without parser or text fallback.
- Language IDs, extensions, grammar fingerprints, and grammar-derived cardinality have one canonical owner.
- Anchored local Grit scans do not reread selected source files solely because text prefiltering and structural evaluation use separate pipelines.
- Existing graph, extraction, GritQL conformance, deterministic-ordering, malformed-source, race, and schema parity tests remain green.
- JavaScript/JSX reuse shared parser and ECMAScript infrastructure; parity work must not introduce duplicate grammar selection or direct application-source parser construction.

Non-goals: the generated GritQL grammar, GritQL compiler, metavariable matcher, containment operators, binding equality, and query/source range domains should not be folded into `parser`. The generated Grit parser handles the query language, not application source, and its size is not duplicate target-language parsing.

## Release-quality gates

Before expanding scope, require:

- [x] Generated diagrams round-trip through their checker.
- [x] Repeated generation is byte-deterministic in current tests.
- [ ] Output is verified deterministic across supported operating systems.
- [x] `--at`, `--related`, and focused flows use one graph and agree through parity tests on declarations, edges, and source ranges across supported focused languages.
- [x] `go test -race ./...` remains green.
- [ ] Benchmarks cover parsing, navigation-index construction, package extraction, workspace extraction, and end-to-end agent retrieval workflows.
- [ ] Performance budgets catch repeated parsing and significant regressions.
- [ ] Public `api` DTO compatibility is tested for the private backend consumer.
- [x] Package and workspace bundles pass canonical drift checks.

## UX improvements after stabilization

- [x] Improve ambiguity diagnostics with suggested `--at PATH:LINE` locations.
- [x] Expose normalized JSON and bounded compact graph output for agent and tool consumption.
- [x] Report every bounded caller/callee omission with counts and complete-graph guidance in human and JSON output.
- [x] Add focused callers/callees/dependencies/dependents/impact graph queries with exact symbol/location and package/module/root-path selectors, bounded depth, cycle protection, and candidate preservation.
- [x] Add repeatable language and confidence filters with normalized query metadata.
- [ ] Add exportedness filtering once declarations expose a reliable cross-language fact; defer edge-kind filtering until the graph contains more than call edges.
- [x] Add repository-wide count summaries independent of output paging.
- [x] Stream validated focused extraction output to stdout when `--output` is omitted.
- [ ] Add a concise package/workspace summary smaller than the generated overview.
- [ ] Evaluate an agent-oriented output profile against the fixed workflow benchmark rather than lowering limits without evidence.
- [x] Emit explicit truncation markers in bounded structures and flows.
- [x] Expose reproducible release/source, revision, commit-time, toolchain, and platform metadata through `--version`.
- [ ] Report the first semantic architecture difference instead of only naming the differing artifact.

## Future optimizations

- Make parser documents the shared cache boundary: allow a document to use either a live Tree-sitter tree or a versioned packed read-only CST through one lightweight node accessor.
- Parse or restore each file once, then derive outlines, navigation graphs, extraction models, and multiple GritQL evaluations from that document.
- Keep packed per-file facts path-neutral and resolve repository context after loading. Invalidate by source digest, language, grammar fingerprint/ABI, schema, and parser/extractor version.

## Language expansion afterward

Add new grammars only after capability reporting and parity work make the current support levels explicit. Swift, Ruby, PHP, and other new adapters should use the shared normalized graph and satisfy the existing definition of done.
