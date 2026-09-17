# Navigation architecture refactor

## Goal

Keep one deterministic parser-owned navigation graph while ensuring that language semantics are implemented by explicit language adapters or indexes rather than language switches and prefix-named helpers in shared orchestration code.

The recently refactored search navigation index is the target pattern:

- one language-neutral graph coordinator;
- one immutable shared corpus;
- registered per-language indexes;
- generic sorting, ambiguity preservation, confidence policy, graph materialization, and traversal;
- language-owned import, re-export, visibility, scope, package/module, and candidate-filter behavior.

Do not replace the shared graph with disconnected per-language graphs.

## Implementation status

Implemented in this checkout:

- repository graph construction now lives in `navigation`;
- `search` uses compatibility wrappers and graph-only related projections;
- `analysis` and `extract` consume `navigation` directly;
- extraction no longer performs language-specific call resolution;
- focused extraction storage uses package/module indexes rather than Go/TypeScript-named buckets;
- focused class and flow lookup plus validation/render semantics are registered on language definitions;
- boundary analysis uses registered language policies;
- GritQL cardinality behavior is adapter-owned;
- architecture tests reject language dispatch in shared extraction and resolver ownership in `search`;
- deterministic text/document and source-order graph parity tests cover the new package;
- parser navigation callbacks are split into language-local files while generic fact helpers remain in `parser/language_navigation.go`.

## Findings

### 1. Focused extraction duplicates navigation resolution

`extract/navigation.go` is effectively a second repository navigation engine.

Language dispatch and standalone language resolvers currently include:

- `navigationDeclarationSymbol`: `extract/navigation.go:49-66`
- `resolveNavigationCallTarget`: `extract/navigation.go:108-133`
- `resolvePythonNavigationCall`: `extract/navigation.go:135-159`
- `resolveJVMNavigationCall`: `extract/navigation.go:161-191`
- `resolveCFamilyNavigationCall`: `extract/navigation.go:193-227`
- generic callable and typed-receiver resolution with Go/non-Go branches: `extract/navigation.go:256-283`
- `resolveImportedGoNavigationSymbol`: `extract/navigation.go:285-290`
- `resolveTypeScriptNavigationModule`: `extract/navigation.go:292-299`
- language-specific call-name and confidence policy: `extract/navigation.go:310-345`

This logic can disagree with graph, `--at`, `--related`, architecture, and boundary resolution.

### 2. `extract.Analysis` exposes historical Go-versus-TypeScript storage

`extract/analyze.go:72-100` contains concrete language indexes:

- `GoDeclarations`
- `GoSymbolIndex`
- Go package/import/fallback maps
- `TSDeclarations`
- `TSSymbolIndex`
- TypeScript module/import/export maps

`TSSymbolIndex` is also used by Python, JVM, C#, Rust, and C-family extraction. It is effectively an incorrectly named “everything except Go” index.

This storage leaks into:

- flow ambiguity: `extract/flow.go:323-359`
- scope and module handling: `extract/scope.go:12-35`, `111-248`, `260-271`
- class lookup and ambiguity: `extract/class.go:1220-1250`, `1416-1448`
- rendering metadata: `extract/generate.go:218-268`
- cross-language dependency selection inside TypeScript-named logic: `extract/ts_generate.go:111-116`

The existing `languageDefinition` abstraction at `extract/language.go:25-39` does not own lookup, scope identity, ambiguity, validation, or rendering policy.

### 3. Generic class validation owns Go and ECMAScript semantics

Representative language-specific ranges in `extract/class.go` include:

- declaration and scope lookup: `1220-1250`
- Go file-local and external-reference validation: `1289-1361`
- member and declaration ambiguity: `1396-1448`
- Go interface and structural-member policy: `1576-1735`

Putting these operations on `classValidator` does not make them language-owned; the generic validator still knows concrete language rules and storage.

### 4. Boundary analysis contains centralized language policy

`search/type_boundaries.go` contains:

- standard-library classification for Go, ECMAScript, Java, Kotlin, and C#: `119-133`
- Go-only third-party import classification: `102-116`
- Go-specific public-container handling: `379-400`

These policies should be registered per language family rather than expanded through another switch.

### 5. GritQL has a side registry outside its language adapter

`targetLanguageAdapter` is defined at `gritql/language.go:40-51`, but Go unfielded-cardinality behavior is separate:

- `unfieldedCardinalityRules`: `gritql/language.go:123-125`
- `goUnfieldedCardinality`: `gritql/language.go:127-131`

`goRootCategoryAccepts` is already correctly installed through the adapter and is not part of this problem.

## Target architecture

### Shared repository navigation engine

Move repository-level graph construction and resolution out of `search` into a reusable package. Parser fact and graph models remain canonical.

Suggested dependency direction:

```text
parser <- navigation <- search
                     <- analysis
                     <- extract
                     <- internal/cli
```

The shared navigation package should own:

- source/document ingestion;
- parser fact merging;
- immutable selected-source corpus construction;
- per-language resolver registration;
- import and re-export resolution;
- call enrichment and candidate constraints;
- shared ambiguity and confidence policy;
- reverse caller indexing;
- deterministic graph materialization;
- source completeness statistics.

`search` should own only search-result and related-result projection. `extract` should consume the resolved graph rather than resolve calls again.

### Focused extraction indexes

Replace the Go/TypeScript split in `Analysis` with a language-neutral scoped index and language-owned projection services.

Possible core representation:

```go
type scopedSymbolKey struct {
    Family string
    Scope  string
    Name   string
}

type focusedIndex struct {
    Declarations map[scopedSymbolKey][]*Declaration
    Symbols      map[scopedSymbolKey][]*Symbol
}
```

Language-owned capabilities should be narrow rather than one large optional interface. Candidate capabilities include:

```go
type focusedScopeResolver interface {
    Scope(*Declaration) string
    ResolveScope(string) []string
}

type focusedSymbolResolver interface {
    ResolveDeclaration(name string, metadata DiagramClass) []*Declaration
    ResolveSymbol(name string, metadata FlowNode) []*Symbol
}

type focusedValidationPolicy interface {
    ValidateDeclaration(*Declaration, DiagramClass) []ValidationIssue
    MembersMatch(required, actual Member) bool
}

type focusedRenderPolicy interface {
    DeclarationMetadata(string, *Declaration, *Analysis) []string
    DeclarationStereotypes(string, *Declaration, *Analysis) []string
}
```

A registered language service bundle can expose only the capabilities it supports.

### Boundary policies

Introduce a registry such as:

```go
type boundaryLanguagePolicy interface {
    ImportOrigin(path string, context boundaryDependencyContext) BoundaryTypeOrigin
    PublicTypeUsage(parser.NavigationDeclaration, string) bool
}
```

Provide implementations for Go, ECMAScript, JVM, C#, and other families only where direct evidence exists. The generic boundary engine should retain aggregation, ranking, sorting, and uncertainty handling.

### GritQL adapter completion

Add unfielded-cardinality behavior directly to `targetLanguageAdapter`:

```go
type targetLanguageAdapter struct {
    // existing fields
    unfieldedCardinality func(parentKind string, hasOpenParen bool) bool
}
```

Use a default permissive implementation for languages without a specialized rule and remove `unfieldedCardinalityRules`.

## Migration plan

### Phase 1: lock behavior

1. Add byte-for-byte graph parity fixtures for representative Go, TypeScript, Python, JVM, C#, Rust, and C/C++ repositories.
2. Cover cold/warm cache parity and permuted source ordering.
3. Add focused structure/flow parity fixtures for ambiguous names, imports, inheritance, receivers, and package/module collisions.
4. Record current source completeness and uncertainty behavior.

Acceptance criteria:

- Existing graph schemas remain unchanged unless the wire contract intentionally changes.
- Ambiguous targets remain ambiguous.
- No new framework, build-system, compiler, or runtime inference is introduced.

### Phase 2: extract the shared repository navigation engine

1. Move the generic graph coordinator and language-index registry from `search` into the shared navigation package.
2. Move `NavigationAnalysis`, document-source inputs, build options, and source statistics with it.
3. Keep temporary compatibility wrappers in `search`.
4. Update `analysis`, CLI graph workflows, ask universes, and directory architecture to call the shared package directly.
5. Preserve one-way dependencies; the public repository must not import backend internals.

Acceptance criteria:

- `search` no longer owns repository graph semantics.
- All existing navigation and architecture tests pass without output changes.

### Phase 3: remove extraction call resolution

1. Build or receive one resolved navigation graph for the complete extraction source universe.
2. Map focused `Symbol` projections to parser declaration IDs.
3. Derive calls from graph `TargetID`, candidate IDs, and confidence.
4. Delete language dispatch from `extract/navigation.go`.
5. Delete `resolvePythonNavigationCall`, `resolveJVMNavigationCall`, `resolveCFamilyNavigationCall`, `resolveImportedGoNavigationSymbol`, and `resolveTypeScriptNavigationModule` once parity is proven.

Acceptance criteria:

- Focused extraction and search use the same resolved call edges.
- `extract` performs no independent repository call resolution.
- Parser-provided ambiguity and visibility decisions are preserved.

### Phase 4: normalize focused extraction storage

1. Introduce language-neutral scoped declaration and symbol indexes.
2. Populate them from every language analyzer.
3. Add registered focused language services for scope and symbol lookup.
4. Migrate Go package maps into a Go focused index.
5. Migrate ECMAScript module/import/export maps into an ECMAScript focused index.
6. Stop placing Python, JVM, C#, Rust, and C-family symbols into `TSSymbolIndex`.
7. Remove the old Go/TS maps after all callers migrate.

Acceptance criteria:

- Generic `Analysis` contains no `Go*` or `TS*` fields.
- Adding a language does not require adding fields to `Analysis`.
- Registry coverage tests align focused languages with declared capabilities.

### Phase 5: move validation, flow, scope, and rendering policy

1. Replace `flowSymbolAmbiguous` language branches with language service lookup.
2. Move Go and ECMAScript scope resolution out of `extract/scope.go`.
3. Replace `classValidator` Go/ECMAScript branches with validation-policy calls.
4. Move Go implementation inference and file-local checks into the Go service.
5. Move export, package/module, underlying-type, struct-tag, and file-local rendering policy behind focused render services.
6. Rename TypeScript-specific generic functions once they no longer process unrelated languages.

Acceptance criteria:

- Shared class, flow, scope, and render files contain no source-language-name switches.
- Language semantics are implemented in language-specific files or receiver types.
- Generic validation remains deterministic and language-neutral.

### Phase 6: boundary policy registry

1. Introduce the boundary policy interface and registry.
2. Move standard-library and third-party classification into language policies.
3. Move Go public-container handling into the Go boundary policy.
4. Preserve unresolved classification when a language lacks sufficient evidence.
5. Add registry coverage and per-language policy tests.

Acceptance criteria:

- `search/type_boundaries.go` contains no language-name switch.
- Unsupported classification remains explicit rather than guessed.

### Phase 7: complete GritQL adapter ownership

1. Add unfielded-cardinality behavior to `targetLanguageAdapter`.
2. Register the Go rule through the Go adapter.
3. Remove the side registry.
4. Verify all GritQL conformance and wrapped-language tests.

### Phase 8: locality and enforcement

1. Split `parser/language_navigation.go` into language-local navigation files while retaining the existing `navigationAdapter` contract.
2. Add architecture tests that reject language-name switches in shared navigation, extraction, boundary, and GritQL orchestration files.
3. Add tests ensuring each declared language capability has its required resolver/service registration.
4. Document allowed exceptions for infrastructure concepts such as shell quoting or Go tooling hooks.

## Non-goals

- Do not create separate per-language navigation graphs.
- Do not duplicate parser extraction semantics in consumers.
- Do not introduce compiler-grade whole-program resolution.
- Do not infer framework/runtime registrations.
- Do not resolve C/C++ compiler include paths without explicit compiler or build evidence.
- Do not guess Rust external crates or conditional module ownership.
- Do not automatically modify external user-installed skills.

## Validation

For every phase, run:

```text
make test
make lint
go vet ./...
go test -race ./...
go mod tidy -diff
git diff --check
```

Also run:

- parser generation checks;
- hook tests;
- backend tests, lint, and `make schema-check` when public contracts or fixtures are affected;
- repeated multi-language graph runs with byte-identical output checks;
- focused structure and flow generation followed by `extract check`.

## Completion criteria

The refactor is complete when:

- one shared engine resolves repository navigation for every consumer;
- `extract` does not resolve calls independently;
- generic analysis structures have no Go/TypeScript-specific index fields;
- shared orchestration contains no source-language-name dispatch except registry lookup or canonical family normalization;
- language capabilities and resolver/service registrations are tested for parity;
- deterministic, cache, ambiguity, completeness, and visibility behavior remains unchanged;
- all completed work is moved to `solved.md`, with only genuinely open work remaining in `next.md`.
