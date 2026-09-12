# Next improvements

Grepple should stabilize for one or two iterations before adding more languages. The recent work introduced a large extraction, navigation, CLI, schema, and hook surface.

## Recommended next milestone

> Expand the usable graph feature set with programmatic output, compact views, queries, and diffs; improve resolution completeness when real workflows expose a blocking ambiguity.

## Feature set versus feature completeness

Treat these as separate planning dimensions:

- **Feature set** determines which workflows Grepple makes possible. New graph projections and operations can unlock entirely new agent, developer, and CI use cases.
- **Feature completeness** determines how precisely an existing workflow handles language and type-system edge cases. Import, receiver, lexical-scope, and return-type resolution belong here.
- Completeness still needs a minimum reliability floor: deterministic candidates, bounded output, no unsafe binding leakage, and actionable ambiguity must remain release gates.
- After that floor is met, prefer a new usable capability over indefinitely refining syntax-based resolution. Return to completeness work when dogfooding demonstrates that a specific ambiguity blocks a valuable workflow.

## Findings from dogfooding

### What is already strong

- The workspace → package overview → `--at` → `--related` workflow is a fast and effective way to enter an unfamiliar codebase.
- Structural search is highly token-efficient because it preserves the enclosing declaration while collapsing unrelated code.
- Exact declaration and member ranges make generated diagrams useful navigation maps rather than passive documentation.
- Focused TypeScript flow is concise and readable for small call chains.
- Canonical package and workspace checks make architecture documentation trustworthy enough to use as an index.
- Deterministic ordering, bounded traversal, explicit confidence, and round-trip validation are strong foundations for agent use.

### Remaining friction

- External LLM dogfooding confirmed the Grepple locate/orient → anchored Edit loop, while also exposing grep-flag muscle memory, edit-target lines omitted by segment limits, broad output reaching the agent harness limit, and the need for anchor-provider diagnostics.
- The shared `parser.NavigationGraph` is now the sole source of generic call edges for Go and TypeScript flow selection, rendering, validation, `--at`, and `--related`; the next graph issue is richer resolution rather than duplicate discovery.
- Larger diagrams are dominated by validation metadata when viewed as plain text.
- Agent edit anchors are provider-specific, not generic line hashes. Grepple delegates through a user-owned, versioned batch provider and keeps editing-harness adapters out of the shipped repository.
- Explicit Go and TypeScript imports, direct parameters/receivers, source-ordered lexical bindings, local and imported return signatures, and same-file typed field chains now resolve before terminal-name fallback. Cross-file field chains, embedded/promoted methods, re-exports, and default imports still need richer propagation.
- Architecture extraction remains richer for Go than TypeScript and other languages.
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

#### C1. Normalized JSON graph output

Expose declarations, source ranges, packages/modules, calls, resolved target IDs, direction, confidence, and truncation as stable JSON rather than requiring tools to parse Mermaid.

Potential examples:

```bash
grepple graph . --json
grepple graph ./internal/cli --entry 'Run' --depth 2 --json
```

Use case: an agent, editor extension, or CI job can consume exact nodes and edges, join them by stable ID, filter candidates by confidence, and retain complete machine-readable output independently of presentation.

#### C2. Compact agent-facing architecture output

Add a compact projection that omits validation metadata and repetitive schema details while preserving source locations, graph identity, edge confidence, and truncation warnings. Canonical Mermaid artifacts remain lossless and self-validating.

Potential examples:

```bash
grepple graph ./internal/cli --entry 'Run' --compact
grepple extract structure ./search --compact
```

Use case: an agent can orient itself in a package or focused call chain within a small token budget, then jump to exact declarations with `--at` without reading a large generated bundle.

#### C3. Graph queries

Support focused questions over the normalized graph: callers, callees, dependencies, dependents, and bounded impact radius. Filters should cover package/module, language, edge kind, exportedness, direction, confidence, and depth.

Potential examples:

```bash
grepple graph callers 'Service.Save' . --depth 2
grepple graph dependencies ./internal/cli --package search
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

### C2. Remove the redundant Read call for edits

1. [x] Add user-owned `~/.grepple/settings.json` anchor-provider configuration; never execute repository-controlled provider commands.
2. [x] Add a versioned whole-file batch protocol with digest checks, timeout/output limits, strict response validation, and direct process execution without a shell.
3. [x] Emit unambiguous `HASH│LINE│content` rows from local structural and `--line-only` output while retaining synthetic summary markers.
4. [x] Verify locally that a user-owned provider can produce anchors accepted by the configured editing harness without shipping harness-specific adapter code.
5. [x] Allow user settings to enable anchors by default for compatible output, with `--no-anchors` as a per-command escape hatch and automatic fallback for incompatible modes.
6. [ ] Add `grepple anchors doctor` (or equivalent) to report provider identity/protocol, test a temporary file, and diagnose stale configuration.
7. [ ] Evaluate anchors for related previews and context output only if dogfooding shows they save additional tool calls; keep JSON provider-neutral.


### D. Expand hardening and performance coverage

1. Grow each language fixture with malformed, nested, generic, decorated/annotated, and multiline declarations.
2. Add CRLF, symlink, build-tag, ambiguous-extension, and Windows-path cases.
3. Run fuzz targets for longer periods in scheduled CI and retain minimized regressions as seeds.
4. Benchmark parse, navigation-index construction, focused extraction, package bundles, and workspace bundles.
   - [x] Add a reproducible warm-page-cache comparison of Tree-sitter parsing, parse-plus-navigation extraction, normalized graph caches, full read-only CST projections, and experimental native `TSTree` serialization. Compare JSON, gob, manual protobuf wire code, standard `protoc-gen-go`, and `vtprotobuf` for message-oriented and string-interned packed layouts; report corpus/cache sizes, throughput, allocations, source-file count, and native serialize/deserialize cost.
   - Next: add focused extraction and package/workspace bundle benchmarks.
5. Add performance budgets that detect repeated parsing and significant allocation/runtime regressions.

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
6. Make parser documents the shared cache boundary.
   - Allow a document to be backed by a live Tree-sitter tree or the production packed read-only CST through the same lightweight node accessor.
   - Parse or restore each file once, then derive outlines, navigation graphs, extraction models, and one or more GritQL rule evaluations from that document.
   - Keep repository-context resolution outside the path-neutral per-file cache.

Recommended implementation order: address targeted TypeScript/TSX placeholder-role gaps as conformance cases expose them, then proceed to cached document backends. Exercising a second grammar before shared acquisition and caching stabilized prevented Go-only assumptions from becoming lower-layer contracts.

Acceptance criteria:

- No feature outside `parser` constructs a parser for supported application source directly; parsing the GritQL query language itself remains an intentional exception.
- One parsed/restored document can feed extraction, navigation, outline, and multiple structural rules without reparsing.
- Language IDs, extensions, grammar fingerprints, and grammar-derived cardinality have one canonical owner.
- Anchored local Grit scans do not reread selected source files solely because text prefiltering and structural evaluation use separate pipelines.
- Existing graph, extraction, GritQL conformance, deterministic-ordering, malformed-source, race, and schema parity tests remain green.
- Benchmarks demonstrate fewer parse invocations and source reads; do not accept a package move that only relocates code without reducing duplicate work.

Non-goals: the generated GritQL grammar, GritQL compiler, metavariable matcher, containment operators, binding equality, and query/source range domains should not be folded into `parser`. The generated Grit parser handles the query language, not application source, and its size is not duplicate target-language parsing.

## Release-quality gates

Before expanding scope, require:

- [x] Generated diagrams round-trip through their checker.
- [x] Repeated generation is byte-deterministic in current tests.
- [ ] Output is verified deterministic across supported operating systems.
- [x] `--at`, `--related`, and focused Go/TypeScript flow use one graph and agree through parity tests on declarations, edges, and source ranges.
- [x] `go test -race ./...` remains green.
- [ ] Benchmarks cover parsing, navigation-index construction, package extraction, and workspace extraction.
- [ ] Performance budgets catch repeated parsing and significant regressions.
- [ ] Public `api` DTO compatibility is tested for the private backend consumer.
- [x] Package and workspace bundles pass canonical drift checks.

## UX improvements after stabilization

- [x] Improve ambiguity diagnostics with suggested `--at PATH:LINE` locations.
- [ ] Add a concise architecture-summary command that does not expose Mermaid validation metadata.
- [x] Emit explicit truncation markers in bounded structures and flows.
- [x] Expose reproducible release/source, revision, commit-time, toolchain, and platform metadata through `--version`.
- [ ] Support graph filtering by package or module, edge kind, exportedness, and direction.
- [ ] Offer normalized JSON graph output for agent and tool consumption.
- [ ] Report the first semantic architecture difference instead of only naming the differing artifact.

## Language expansion afterward

Resume the language roadmap only after baseline hardening and graph parity are complete. Swift, Ruby, PHP, and other new adapters should use the shared normalized graph and satisfy the existing definition of done.
