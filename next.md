# Next improvements

Grepple should stabilize for one or two iterations before adding more languages. The recent work introduced a large extraction, navigation, CLI, schema, and hook surface.

## Recommended next milestone

> Complete graph unification, improve resolution confidence, and make large results easier to consume.

## Findings from dogfooding

### What is already strong

- The workspace → package overview → `--at` → `--related` workflow is a fast and effective way to enter an unfamiliar codebase.
- Structural search is highly token-efficient because it preserves the enclosing declaration while collapsing unrelated code.
- Exact declaration and member ranges make generated diagrams useful navigation maps rather than passive documentation.
- Focused TypeScript flow is concise and readable for small call chains.
- Canonical package and workspace checks make architecture documentation trustworthy enough to use as an index.
- Deterministic ordering, bounded traversal, explicit confidence, and round-trip validation are strong foundations for agent use.

### Remaining friction

- External LLM dogfooding confirmed the intended Grepple locate/orient → Read exact range → Edit loop, but also exposed grep-flag muscle memory, edit-target lines omitted by segment limits, and broad output reaching the agent harness limit.
- The shared `parser.NavigationGraph` is now the sole source of generic call edges for Go and TypeScript flow selection, rendering, validation, `--at`, and `--related`; the next graph issue is richer resolution rather than duplicate discovery.
- Larger diagrams are dominated by validation metadata when viewed as plain text.
- Explicit Go and TypeScript imports plus direct parameter and method-receiver types now resolve before terminal-name fallback. Inferred locals, embedded/promoted methods, re-exports, default imports, and multi-hop member chains still need richer binding propagation.
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
- [x] Report matching lines omitted by `--max-segments` and direct edit-oriented searches to `--line-only` followed by Read for hash anchors.
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
   - Next: propagate inferred local-variable, field/member-chain, embedded/promoted-method, re-export, and default-import bindings.
3. [x] Distinguish exact, context-resolved, unique-terminal, and candidate confidence.
4. [x] Rank unresolved candidates deterministically and suggest `--at PATH:LINE` locations.
5. Add fixtures for same-name package functions, methods, interfaces, overloads, inheritance, TypeScript aliases, and TS/TSX imports.

### C. Add compact agent-facing output

1. Keep generated Mermaid lossless and self-validating.
2. Add a compact view or summary command that omits validation metadata from presentation.
3. Preserve source ranges, node identity, edge confidence, and truncation warnings.
4. Add normalized JSON graph output so agents do not need to parse Mermaid.
5. Test that compact and JSON projections come from the same graph as canonical Mermaid.

### D. Expand hardening and performance coverage

1. Grow each language fixture with malformed, nested, generic, decorated/annotated, and multiline declarations.
2. Add CRLF, symlink, build-tag, ambiguous-extension, and Windows-path cases.
3. Run fuzz targets for longer periods in scheduled CI and retain minimized regressions as seeds.
4. Benchmark parse, navigation-index construction, focused extraction, package bundles, and workspace bundles.
5. Add performance budgets that detect repeated parsing and significant allocation/runtime regressions.

### E. Improve architecture drift diagnostics

1. Compare normalized manifests before byte-level artifact comparison.
2. Report the first changed declaration, member, route, relation, package, or module.
3. Keep the final byte comparison as the canonical determinism check.

## Release-quality gates

Before expanding scope, require:

- [x] Generated diagrams round-trip through their checker.
- [x] Repeated generation is byte-deterministic in current tests.
- [ ] Output is verified deterministic across supported operating systems.
- [ ] `--at`, `--related`, and focused flow use one graph and agree exactly on declarations, edges, and source ranges.
- [x] `go test -race ./...` remains green.
- [ ] Benchmarks cover parsing, navigation-index construction, package extraction, and workspace extraction.
- [ ] Performance budgets catch repeated parsing and significant regressions.
- [ ] Public `api` DTO compatibility is tested for the private backend consumer.
- [x] Package and workspace bundles pass canonical drift checks.

## UX improvements after stabilization

- [ ] Improve ambiguity diagnostics with suggested `--at PATH:LINE` locations.
- [ ] Add a concise architecture-summary command that does not expose Mermaid validation metadata.
- [x] Emit explicit truncation markers in bounded structures and flows.
- [x] Expose reproducible release/source, revision, commit-time, toolchain, and platform metadata through `--version`.
- [ ] Support graph filtering by package or module, edge kind, exportedness, and direction.
- [ ] Offer normalized JSON graph output for agent and tool consumption.
- [ ] Report the first semantic architecture difference instead of only naming the differing artifact.

## Language expansion afterward

Resume the language roadmap only after baseline hardening and graph parity are complete. Swift, Ruby, PHP, and other new adapters should use the shared normalized graph and satisfy the existing definition of done.
