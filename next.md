# Next improvements

Grepple should stabilize for one or two iterations before adding more languages. The recent work introduced a large extraction, navigation, CLI, schema, and hook surface.

## Recommended next milestone

> Stabilize command discoverability, diagnostic fidelity, boundary-signal classification, and reusable analysis caches before adding languages. Then prioritize richer cross-file resolution where dogfooding shows that ambiguity blocks a valuable workflow.

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
- Focused flow is concise and readable for small call chains across Go, ECMAScript, Python, Java, Kotlin, and C# adapters.
- Canonical package and workspace checks make architecture documentation trustworthy enough to use as an index.
- Deterministic ordering, bounded traversal, explicit confidence, and round-trip validation are strong foundations for agent use.
- `--count-summary` now reports complete matched-file and matching-line breadth independently of paging, while `--count` retains grep-compatible per-file page output.

### Remaining friction

- External LLM dogfooding confirmed the Grepple locate/orient → Edit loop, while also exposing grep-flag muscle memory, edit-target lines omitted by segment limits, and broad output reaching the agent harness limit.
- The shared `parser.NavigationGraph` is now the sole source of generic call edges for flow selection, rendering, validation, `--at`, and `--related`; the next graph issues are focused querying and richer resolution rather than duplicate discovery.
- `--related` now reports omitted callers and callees, but its continuation guidance still points at a broad complete graph instead of a copyable focused callers/callees query.
- Broad structural output now defaults to 16384 bytes after the 800-match benchmark measured a 60% token reduction; complete JSON and the explicit uncapped override remain available.
- Generated package overviews can still be too large for focused questions: the extract package overview reached 526 lines/27 KB, while a file outline answered the immediate JVM question in 50 lines/1.9 KB.
- Larger diagrams are dominated by validation metadata when viewed as plain text.
- Agent edit anchors are provider-specific, not generic line hashes. Grepple delegates through a user-owned, versioned batch provider and keeps editing-harness adapters out of the shipped repository.
- Explicit Go and TypeScript imports, direct parameters/receivers, source-ordered lexical bindings, local and imported return signatures, and same-file typed field chains now resolve before terminal-name fallback. Cross-file field chains, embedded/promoted methods, re-exports, and default imports still need richer propagation.
- Architecture extraction now covers the high-value object models through C#; C/C++, Rust, and Shell still need useful non-class projection contracts before focused extraction is enabled.
- Cross-language fixtures prove the baseline but do not yet cover enough malformed, nested, generic, decorated, or multiline syntax.
- Canonical mismatch errors identify the artifact but should eventually report the first semantic difference.

## Current Grepple tool review

This review treats Grepple as both a developer CLI and an agent tool. The strongest parts are the workflows that compress source while preserving exact locations; the weakest parts are command discovery, consistency between modes, and distinguishing architectural evidence from architectural risk.

### Where Grepple is most useful

- **Repository orientation:** `extract summary workspace|package`, outlines, and canonical architecture artifacts reveal ownership and dependency direction much faster than reading directory trees or manifests.
- **Bounded code retrieval:** structural search normally returns the enclosing declaration and collapses unrelated bodies. It is substantially more useful than raw matching lines for understanding code while remaining small enough for an agent context.
- **Edit targeting:** `--line-only`, `--enclosing`, `--at`, exact source ranges, and optional anchors form a good locate → understand → edit path. The location notation composes well across commands.
- **Impact exploration:** `--related`, graph queries, confidence labels, candidate preservation, and omitted-edge counts make syntax-based navigation honest enough for investigation when treated as evidence rather than a compiler call graph.
- **Machine integration:** deterministic ordering, stable graph IDs, complete JSON, explicit schemas, semantic graph diffs, and canonical checks make the tool suitable for CI and other programs instead of only terminal use.
- **Structural querying:** native bounded GritQL across every parser-backed language is a substantial capability, especially because unsupported constructs fail instead of silently degrading to text matching.
- **Operational safety:** local-first behavior, explicit remote use, bounded human output, cancellation and resource limits, ignored-file handling, and user-owned anchor providers are appropriate defaults for agent execution.
- **Capability honesty:** `grepple languages` clearly exposes the uneven feature matrix instead of implying that every language supports every projection.

### Main downsides and friction points

#### Command discovery and help

- The root help presents only the default text-search interface and does not list `grit`, `graph`, `boundaries`, `extract`, `languages`, remote repository commands, authentication, rules, or version discovery. A new user cannot discover most of Grepple from `grepple --help`.
- `grepple help` is interpreted as a search for the word `help`, not as help. Top-level command words such as `graph`, `grit`, and `extract` also collide with legitimate search patterns unless users know to force search mode with an option such as `-F`.
- Nested help is inconsistent. `extract --help` prints only a terse usage line and exits with an error; `extract summary --help` reports a missing mode; `extract summary package --help` treats `--help` as a path. `extract check --help` has the same shallow behavior.
- `grepple anchors --help` falls back to root search help because there is no anchors command, even though anchor configuration is a meaningful subsystem.
- `graph` usage renders `--json` and `--compact` as optional even though exactly one is required. The error is correct, but the discoverable usage contract is not.
- Help pages are mostly option inventories. They need one or two task-oriented examples and explicit statements about output completeness, source-universe scope, and selector semantics.

#### CLI consistency and grep compatibility

- Output mode conventions vary by command: search has human, `--json`, and `--json-matches`; graph requires exactly one of JSON or compact; boundaries defaults to human; extract defaults to Mermaid; GritQL has human or JSON. These choices are individually defensible but collectively expensive to memorize.
- Limit names have different units: search `--limit` pages files, GritQL pages findings, boundaries limits each human section, while `--max-files`, `--max-segments`, `--max-nodes`, and `--max-output-bytes` bound other stages. Every result should disclose all active bounds in one consistent footer or metadata object.
- `-l` means recursive file listing, whereas grep users expect matching filenames. `--files-with-matches` is available, but this is a recurring muscle-memory trap.
- JavaScript regular expressions are the default even in Go-centric repositories. This is useful for compatibility with the existing query engine but surprising to users expecting POSIX or RE2 syntax.
- The optional positional `PATTERN [PATH...]` shape makes path-only intent and command-name searches less obvious than an explicit `grepple search` form would.
- Local and remote modes do not support the same structural/navigation features. The help should show capability differences at the attempted operation, not require prior knowledge of the architecture.

#### Search and retrieval

- Structural output is excellent when its selected segment is the desired unit, but the ranking and segment-selection rationale is opaque. When a useful match is omitted, users know that truncation happened but not why one segment ranked above another.
- Broad searches can still spend the entire 16 KiB budget on low-value early paths because deterministic path order is not relevance order. Count-summary → files → focused retrieval is effective but currently learned through skills/documentation rather than the CLI.
- `--related` currently advises users to inspect a complete `graph --json PATH` when edges are omitted. The more actionable continuation is a focused `graph callers|callees --at PATH:LINE --depth N --json|--compact SCOPE` command.
- Anchor setup remains external and hard to diagnose. Without a doctor command, users must distinguish unsupported output modes, missing providers, provider protocol failures, digest failures, and harness incompatibility manually.

#### Navigation and graph analysis

- Navigation is syntax-based and deliberately conservative. Interfaces, overloads, inheritance, re-exports, default imports, TS/TSX path aliases, cross-file field chains, and promoted Go methods remain important ambiguity sources.
- The selected paths define the graph universe, but this completeness boundary is easy to overlook. Compact and JSON query headers should report discovered, parsed, unsupported, failed, and truncated files consistently.
- Exact `--symbol` selection assumes the user already knows Grepple's normalized declaration name. There is no lightweight selector-resolution command that previews exact and ambiguous candidates before building a traversal.
- Graph and related operations rebuild the repository-local graph for each invocation. Boundary analysis has a content-addressed cache, but ordinary graph queries and repeated `--related` exploration do not yet share it.
- `dependencies` and `dependents` can sound like build/import dependency queries even though the graph is primarily callable navigation. Naming or help must make the edge domain explicit.
- Compact graph output is useful only when rooted or tightly scoped. A whole-package compact dump can still be a large inventory with less signal than an outline plus one focused query.

#### Boundary analysis

- The current `External` flag means “has an import path,” not “third-party dependency.” It conflates standard-library, first-party cross-package, and actual external-module types.
- Type spread is evidence, not leakage. A package-wide private syntax abstraction such as `syntaxNode` can be healthy, while one raw third-party node outside an approved backend can be a serious violation. Current breadth thresholds cannot express that distinction.
- The report needs explicit origin classes such as local, first-party, standard-library, third-party, and unresolved, followed by package-internal, cross-package, cross-layer, and public-API spread.
- Approved containment zones and facade-bypass checks are absent from the generic analyzer. The parser currently relies on a bespoke architecture test to keep Tree-sitter types in approved backend files.
- Test-only framework types, standard-library utility types, adapter protocols, declarative configuration, and benchmark helper workflows create substantial noise.
- Internal types should not be ranked as risks merely because they are popular. Rank them when they cross a declared boundary, expose representation publicly, have an ambiguous owner, or form a competing abstraction.
- Workflow candidates identify repeated topology but do not yet classify utility hubs, declarative setup, lifecycle protocols, or likely misplaced functions. Findings still require considerable manual interpretation.

#### Architecture extraction and checking

- Canonical Go package/workspace bundles are trustworthy and useful, but they are intentionally Go-only while focused projections cover a different language subset. The distinction should be visible in command-specific errors and examples.
- Full Mermaid and manifest artifacts are too large for many questions and contain validation metadata that overwhelms plain-text inspection. Summaries help, but users need clearer guidance on when to use summary, overview, structure, compact graph, or focused extraction.
- Canonical checks report which artifact differs but not the first semantic declaration, relation, route, package, or module difference.
- Direct generation and checking commands exist, but repository-wide canonical workflows still depend on non-discoverable mode combinations and repository-specific Make targets. Commands should explain canonical output locations and next actions without encouraging manual edits.

#### GritQL

- GritQL exposes powerful resource controls but its help is dominated by limits and offers no minimal query examples. First-use success depends heavily on separate compatibility documentation.
- There is no obvious compile-only or explain mode that shows the selected language adapter, wrapper interpretation, metavariable roles, or why a snippet cannot be represented.
- Query diagnostics should consistently include source ranges, attempted wrapper categories, compatibility contract, and a concise remediation path without requiring debug tests.

### Prioritized improvement backlog from this review

#### P0 — make existing capabilities discoverable and trustworthy

- [x] Split broad Grepple guidance into short intent-triggered skills for local retrieval, architecture lookup, change impact, boundary review, structural audits, diagram generation/validation, and remote inspection. Each skill states what its evidence can and cannot prove.

1. [ ] Add top-level command help that lists search, GritQL, graph, boundaries, extraction, languages, rules, repository/authentication commands, and version. Make `grepple help [COMMAND ...]` real while preserving an explicit `grepple search` command for command-word patterns.
2. [ ] Make recursive help work for every extract and graph mode. Usage should encode required output-mode exclusivity and return success for valid help requests.
3. [ ] Classify boundary types as local, first-party, standard-library, third-party, or unresolved. Keep imported identity separate from dependency origin.
4. [ ] Add boundary risk/reason fields rather than presenting breadth as implied leakage. Prioritize public third-party exposure, containment escape, facade bypass, and cross-layer representation spread; mark broad approved internal abstractions as informational.
5. [ ] Replace omitted-edge guidance with a copyable focused graph-query continuation using the current location, direction, scope, and an appropriate complete or compact mode.
6. [ ] Audit every local structural/graph operation for parse failures and unsupported files; expose deterministic parsed/skipped/failed/truncated totals in human and JSON output.

#### P1 — reduce repeated user and agent work

1. [ ] Reuse content- and grammar-addressed per-file navigation facts across graph, graph query, `--related`, boundary analysis, and extraction while keeping output independent of cache state.
2. [ ] Add selector preview/disambiguation, for example `graph resolve --symbol NAME --compact SCOPE`, returning exact declaration IDs and actionable `--at` alternatives without traversing the graph.
3. [ ] Add `anchors doctor` with provider identity, protocol version, temporary-file round trip, timeout/output diagnostics, and clear setup instructions.
4. [ ] Standardize output metadata and terminology across search, graph, boundaries, and GritQL: selected scope, completeness, paging, byte caps, source caps, omitted counts, and a copyable next command.
5. [ ] Add task-oriented examples to command help: orient, locate, edit, inspect callers, check impact, run one GritQL query, generate a focused diagram, and validate a canonical bundle.
6. [ ] Explain search ranking and add an opt-in relevance strategy that remains deterministic. Keep path order available for reproducible scripts.
7. [ ] Add a compile-only/explain path for GritQL queries with wrapper selection, metavariable interpretation, language compatibility, and bounded diagnostics.

#### P2 — improve precision and reduce conceptual surface

1. [ ] Complete the documented cross-file member, import/re-export, alias, interface, overload, inheritance, and promoted-method resolution cases based on measured ambiguity frequency.
2. [ ] Decide and document the distinct lifecycle contracts for `Document`, `Node`, `DocumentView`, `ViewNode`, and `SyntaxNode`; deprecate a surface if two provide the same job without a measurable safety or performance difference.
3. [ ] Add generic containment policies and facade-bypass analysis using repository-owned configuration or generated architecture ownership facts. Keep language/framework defaults optional to avoid brittle hard-coding.
4. [ ] Extend boundary surfaces to fields/properties and distinguish public API, private signature, field representation, and body-local usage.
5. [ ] Report the first semantic architecture difference and provide a focused source-linked diff before the final byte-level mismatch.
6. [ ] Define useful focused architecture contracts for Rust and then C/C++ only after real dogfood questions demonstrate what should be projected.

### Review success measures

- A new user can discover and run search, graph impact, boundaries, focused extraction, and one GritQL query using only recursive CLI help.
- An agent can move from an omitted `--related` edge to a complete focused query by copying one suggested command.
- Boundary output separates true third-party permeability from first-party API reuse and standard-library noise, with reviewed precision fixtures.
- Warm repeated graph queries parse or restore each unchanged file once and remain byte-identical to cold output.
- Every bounded response states what was bounded, what was omitted, and whether JSON or a narrower command provides completeness.
- Dogfood benchmarks track task success and retrieval turns in addition to bytes, tokens, runtime, and allocations.

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
- [x] Enrich plain `--line-only` locations with parser-backed `PATH:START-END` ranges when the matching line begins a multi-line declaration or control-flow construct, plus opt-in `--enclosing` `PATH:MATCH@START-END` ranges for body matches; preserve single-line and exact anchor-row contracts.
- [x] Report related-edge truncation explicitly, including omitted caller/callee counts in human and JSON output plus complete-graph guidance. Tests prove bounded previews cannot be mistaken for complete impact results.
- [x] Add `--count-summary` for complete aggregate matched-file and matching-line totals independent of `--skip`, `--limit`, and `--max-files`; retain `--count-by-repo` for grouped compatibility output.
- [x] Lower the default human-output budget from 40960 to 16384 bytes after a synthetic 800-match benchmark showed a 60% output/token reduction with no effect on bounded workflows or complete JSON.
- [x] Stream validated `extract structure|flow` projections to stdout when `--output` is omitted.
- [x] Add bounded `extract summary package|workspace` Markdown projections from canonical Go IR. On this checkout the package summary reduced extract orientation from 27.5 KB to 2.1 KB, while the workspace summary reduced 4.3 KB to 2.4 KB.
- [x] Provide a compact presentation that omits Mermaid validation metadata while retaining complete canonical bundles for detailed or machine-readable use.

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

Boundary pattern analysis follow-up:
- [x] Add language-neutral external consumer breadth, callable surface, callable co-usage, ordered sequences, and member-read/member-write plus call combinations.
- [x] Expose directory-ranked analysis as `grepple boundaries [PATH...]`: group resolved target declarations by owner file, remove owner-local calls, require repeated workflows across at least two external files, and report actionable caller locations.
- [x] Cache content- and grammar-addressed resolved navigation graphs under ignored `.grepple/cache/boundaries/` for fast unchanged rechecks.
- [x] Report import-qualified external and unambiguously owned project type spread with usage roles, public-signature exposure, production/test reach, and deterministic evidence locations.
- [ ] Extend external surface totals from callables to fields/properties once every adapter exposes declaration ownership with honest confidence.

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

[x] `grepple graph --compact [PATH...]` emits declarations and directed calls from the same normalized graph, retaining stable ID prefixes, source locations, confidence, candidate identities, and file-truncation warnings while omitting JSON and Mermaid validation detail. Human output is bounded by the shared 16384-byte default; JSON remains complete and uncapped.

Potential examples:

```bash
grepple graph --compact ./internal/cli
grepple graph --compact ./search --max-output-bytes 20000
```

Use case: an agent can orient itself in a package or focused call chain within a small token budget, then jump to exact declarations with `--at` without reading a large generated bundle.

#### C3. Graph queries

- [x] Add deterministic `graph callers`, `callees`, `dependencies`, `dependents`, and bidirectional `impact` traversal with bounded depth, cycle protection, candidate-edge preservation, complete JSON, and bounded compact output. Exact symbol/location selectors choose one root; package/module/root-path selectors choose a scope.
- [x] Add repeatable canonical language and confidence filters before root selection and traversal; preserve normalized filter metadata in JSON and compact output.
- [x] Add explicit `public`, `non-public`, and `unknown` declaration visibility facts plus repeatable pre-traversal `--visibility` graph filtering. Unknown preserves honesty for languages without reliable visibility semantics. Defer edge-kind filtering until normalized graphs contain more than call edges.

Potential examples:

```bash
grepple graph callers --symbol 'Service.Save' --language go --confidence exact --depth 2 --compact .
grepple graph callees --at internal/cli/local.go:120 --depth 2 --json .
grepple graph dependencies --root-path internal/cli --depth 1 --compact .
grepple graph impact --at internal/cli/local.go:120 --depth 3 --compact
```

Use case: before editing a declaration, an agent or developer can identify likely consumers, affected packages, and relevant tests without manually traversing repeated `--related` responses.

#### C4. Architecture and graph diffing

[x] Compare two normalized source graphs and report added, removed, moved, and semantically changed declarations and call edges through `grepple-navigation-diff-v1`. Position-only shifts are ignored; human output is bounded and JSON complete.

Potential examples:

```bash
grepple graph diff --before ../old-tree --after . --compact
grepple graph diff --before ../old-tree --after . --json
```

Use case: code review and CI can explain architectural impact—such as a new package dependency, removed route, or changed caller edge—rather than only reporting that a generated artifact differs.

#### C5. Shared-projection guarantees

1. Derive compact text, JSON, graph queries, diffs, and canonical Mermaid from the same normalized graph.
2. Preserve stable node identity, source ranges, edge confidence, and truncation semantics in every applicable projection.
3. [x] Add an end-to-end parity fixture proving complete JSON IDs/edges, compact IDs/edges, focused Mermaid flow, canonical package Mermaid declarations/ranges, and `--related` agree.

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
4. [x] Benchmark parse, navigation-index construction, focused extraction, package bundles, and workspace bundles.
   - [x] Add a reproducible warm-page-cache comparison of Tree-sitter parsing, parse-plus-navigation extraction, normalized graph caches, full read-only CST projections, and experimental native `TSTree` serialization. Compare JSON, gob, manual protobuf wire code, standard `protoc-gen-go`, and `vtprotobuf` for message-oriented and string-interned packed layouts; report corpus/cache sizes, throughput, allocations, source-file count, and native serialize/deserialize cost.
   - [x] Add deterministic focused structure/flow and package/workspace bundle benchmarks with output-size metrics and reviewed runtime/allocation budgets in `docs/architecture-performance-benchmarks.md`.
5. [x] Add reviewed performance budgets for repeated parsing and significant allocation/runtime regressions; keep host-sensitive thresholds as benchmark review gates rather than flaky unit assertions.
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
   - [x] Move callable, container, declaration, call, import, binding, return-type, and visibility syntax policy behind `languageAdapter.Navigation()`; the generic navigation collector now depends on adapters rather than a language string.
   - [x] Add AST architecture guards that reject canonical language IDs and grammar node-kind literals in generic navigation engines and verify every registered adapter supplies parsing and navigation semantics.
   - [x] Make `languageAdapter.Parse()` the parser entry point, hide Tree-sitter grammars, trees, nodes, and cursors behind private syntax handles, remove the raw-tree public graph API, and reject production go-tree-sitter imports outside the syntax backend.
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
   - [x] Add C# focused structure/flow with classes, interfaces, structs, records, enums, inheritance, properties, fields, constructors, methods, visibility, static members, project-root discovery, and shared navigation resolution. Keep C/C++, Rust, and Shell unsupported until their non-class architecture projections have an explicit useful contract.
Recommended implementation order from here: improve cross-file member and TypeScript import/re-export resolution, expand ambiguity fixtures, and define useful non-class projections for Rust or C/C++ before expanding focused extraction.

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
- [x] Benchmarks cover parsing, navigation-index construction, focused extraction, package extraction, workspace extraction, and end-to-end agent retrieval workflows.
- [x] Reviewed performance budgets identify repeated parsing and significant allocation/runtime regressions without introducing host-sensitive unit-test failures.
- [ ] Public `api` DTO compatibility is tested for the private backend consumer.
- [x] Package and workspace bundles pass canonical drift checks.

## UX improvements after stabilization

- [x] Improve ambiguity diagnostics with suggested `--at PATH:LINE` locations.
- [x] Expose normalized JSON and bounded compact graph output for agent and tool consumption.
- [x] Report every bounded caller/callee omission with counts and complete-graph guidance in human and JSON output.
- [x] Add focused callers/callees/dependencies/dependents/impact graph queries with exact symbol/location and package/module/root-path selectors, bounded depth, cycle protection, and candidate preservation.
- [x] Add repeatable language and confidence filters with normalized query metadata.
- [x] Add explicit declaration visibility facts and repeatable `--visibility public|non-public|unknown` filtering before root selection and traversal; defer edge-kind filtering until the graph contains more than call edges.
- [x] Add repository-wide count summaries independent of output paging.
- [x] Stream validated focused extraction output to stdout when `--output` is omitted.
- [x] Add concise bounded package/workspace summaries from canonical IR, substantially smaller than generated overviews on the dogfood checkout.
- [x] Lower the default human-output budget to 16384 bytes after the fixed broad-output workflow measured a 60% token reduction without affecting complete JSON.
- [x] Emit explicit truncation markers in bounded structures and flows.
- [x] Expose reproducible release/source, revision, commit-time, toolchain, and platform metadata through `--version`.
- [ ] Report the first semantic architecture difference instead of only naming the differing artifact.

## Future optimizations

- Make parser documents the shared cache boundary: allow a document to use either a live Tree-sitter tree or a versioned packed read-only CST through one lightweight node accessor.
- Parse or restore each file once, then derive outlines, navigation graphs, extraction models, and multiple GritQL evaluations from that document.
- Keep packed per-file facts path-neutral and resolve repository context after loading. Invalidate by source digest, language, grammar fingerprint/ABI, schema, and parser/extractor version.

## Language expansion afterward

Add new grammars only after capability reporting and parity work make the current support levels explicit. Swift, Ruby, PHP, and other new adapters should use the shared normalized graph and satisfy the existing definition of done.
