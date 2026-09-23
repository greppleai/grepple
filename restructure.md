# Repository restructuring plan (completed historical record)

## Goal

Move the repository toward a layered structure in which reusable analysis and source-processing capabilities live below the CLI, command packages are terminal adapters, transport contracts do not define core behavior, and the shared command runtime remains small and invocation-neutral.

The restructuring must preserve command output, remote/local parity, explicit source-range behavior, source statistics, and existing public APIs unless a compatibility decision is made explicitly.

## Current dependency shape

The healthy core direction is broadly:

```text
parser <- navigation <- search <- analysis
                         ^          ^
                         |          |
                      commands and service adapters
```

The main remaining problems are:

1. `analysis` and `internal/cli/{architecture,graph}` still retain transitional report builders and Architecture Compare remains command-owned.
2. Core engines use HTTP request and response types from `api` as internal domain models.
3. Source discovery and policy are spread across Search, Extract, `sourcepolicy`, `sourceinspection`, and Directory Metadata; `sourcepolicy` still adapts Search and Extract directly.
4. `parser`, `navigation`, `search`, and `extract` overlap around navigation facts, graph operations, language semantics, and source discovery.
5. `cmd/grepple` configures Parser cache policy directly.
6. Deprecated package-level command wrappers may preserve unnecessary entry points and dependencies.
7. Ask and Init resolve the same AI model preference through separate command-owned implementations.
8. A few one-consumer helper packages have speculative or duplicated ownership and need explicit keep/merge decisions.

## Target dependency rules

The completed structure should obey these rules:

```text
cmd/grepple
    -> internal/cli
        -> internal/cli/<command>
            -> internal/cliruntime
            -> reusable engines
            -> internal/render where required

internal/cli/<command> -X-> internal/cli/<other-command>
internal/cliruntime    -X-> command packages
internal/cliruntime    -X-> render
reusable engines       -X-> internal/cli
core domain models     -X-> api transport DTOs
```

Additional rules:

- `internal/cli` is only the composition and dispatch root.
- Every command package owns argument parsing, command workflow, and command-specific rendering.
- Reusable parsing, selection, graph, architecture, and analysis behavior must not be owned by command packages.
- `cliruntime.Context` contains only streams, API access, immutable invocation configuration, repository facts, and exit signaling.
- `sourcepolicy` represents policy. It must not become a facade over Search or Extract.
- `api` owns wire compatibility and serialization, not core algorithms.
- Core packages should form an acyclic dependency graph.
- New packages should be introduced only when they establish a real ownership boundary, not merely to reduce file counts.

## Proposed structure

This is a directional target rather than a requirement to create every directory immediately:

```text
analysis/
    universe.go
    architecture.go
    graph.go
    responsibilities.go
    compare.go

parser/
    syntax and language-owned facts

navigation/
    graph construction and symbol/dependency resolution

search/
    text/structural query execution, target selection, ranking, and related-result projection

extract/
    focused semantic structure/flow and Mermaid validation/generation

api/
    HTTP request/response contracts and conversion adapters

internal/
    cliruntime/
        command.go
        context.go
        configuration.go
        repository.go

    sourcepolicy/
        policy.go              # neutral immutable options and bypass notices only

    sourcecatalog/
        discovery.go           # gitignore-aware deterministic source discovery
        decisions.go           # selected/excluded decisions and classifications

    sourceselector/            # only if selectors are shared by multiple commands
        selector.go

    anchor/                    # only if anchor calculation is reusable outside rendering
        anchor.go

    cli/
        application.go
        args.go
        <command>/
            command.go
            args.go
            local.go
            remote.go
            render.go

    render/
        shared presentation primitives only
```

Package names and exact files may differ. Dependency direction and ownership are the acceptance criteria.

## Migration plan

### Phase 0: establish a trustworthy baseline

- [x] Preserve the current uncommitted work before moving files or deleting wrappers.
- [x] Run `go test ./...` and `git diff --check`.
- [x] Generate a production-only directory graph so tests do not create false architecture edges:

  ```bash
  go run ./cmd/grepple --production-only architecture directory \
    --mermaid --depth 0 --max-nodes 0 \
    --output architecture-production.mmd .
  ```

- [x] Save compact production relations for before/after comparison.
- [x] Record existing local/remote parity tests and CLI golden or contract tests that protect output.
- [x] Treat unresolved, ambiguous, and parser-recovered relation counts as completeness qualifications; absence of an edge is not proof of independence.

Exit criteria:

- Baseline tests pass.
- A production-only architecture artifact exists.
- The large preexisting diff is understood well enough that restructuring changes can be reviewed separately.

### Phase 1: make `analysis` the single analysis owner

Consolidate duplicated Architecture and Graph behavior before extracting smaller utilities.

- [x] Compare `analysis.ArchitectureReport` with `internal/cli/architecture.Report` field by field.
- [x] Compare `analysis.GraphReport` with `internal/cli/graph.Output` and their query/truncation models.
- [x] Select the canonical schemas and document any intentional local/remote differences.
- [x] Move reusable source loading and parsed-universe construction behind `analysis` APIs where it belongs.
- [x] Change the Architecture command to call `analysis.BuildArchitecture` rather than constructing directories, symbols, and relations itself.
- [x] Change the Graph command to call `analysis.BuildGraph` rather than owning graph projection logic.
- [x] Move reusable Responsibilities computation into `analysis`; leave command parsing and rendering in the Architecture command package.
- [x] Move reusable Architecture Compare normalization and semantic comparison into `analysis`; leave snapshot I/O, byte-level encoding diagnostics, exit signaling, and text/JSON rendering in the command package.
- [x] Keep CLI argument parsing, output limits, spill behavior, and `--output` handling in the CLI package.
- [x] Add local/remote parity tests around the canonical reports.
- [x] Delete duplicate CLI report builders after all callers use the canonical implementation (`architecture.BuildFromParts`, `graph.OutputFromParts`, and `graph.FromParts`).

Expected result:

```text
internal/cli/architecture -> analysis
internal/cli/graph        -> analysis
analysis                  -X-> internal/cli/*
```

Exit criteria:

- One implementation builds architecture relations.
- One implementation builds graph reports.
- Architecture and Graph command output remains byte-compatible unless a versioned output change is approved.
- Existing explicit range and source-statistics tests still pass.

### Phase 2: remove lateral command dependencies

For each command-to-command edge, identify the actual reusable capability and move only that capability.

#### Ask

- [x] Replace Ask imports of Architecture and Graph command packages with calls to `analysis`.
- [x] Replace Ask imports of Search and Grit command packages with engine-level APIs or small command-independent workflows.
- [x] Move indexed-reference filtering to command-neutral `internal/repositoryrefs`.
- [x] Move source-scope inspection to command-neutral `internal/sourceinspection`.
- [x] Keep Ask-specific tool descriptions, request shaping, result budgeting, and tool rendering inside Ask.
- [x] Do not create one large shared “services” object for Ask.

#### Boundaries

- [x] Replace `internal/cli/boundaries -> internal/cli/graph` with an `analysis` or `navigation` API.
- [x] Keep boundary heuristics in their canonical analysis owner and boundary rendering in the command package.

#### Graph and Extract

- [x] Move generic `PATH[:LINE[-END]]` parsing out of the Extract command package.
- [x] Introduce `internal/sourceselector` only if at least two non-command owners need the same selector semantics.
- [x] Preserve exact range semantics: `PATH:START-END` must retain the entire requested range, while `PATH:LINE` may resolve structurally.

#### Search and Anchors

- [x] Separate reusable anchor provider invocation, response validation, native hashing, and direct-read generation into command-neutral `internal/anchor`.
- [x] Keep Search-specific result projection and source-change validation in `internal/cli/search`.
- [x] Keep anchor command parsing, provider setup, diagnostics, and rendering in `internal/cli/anchors`.
- [x] Avoid moving Search-specific result models into the neutral anchor package.

Exit criteria:

- No production `internal/cli/<command>` package imports another command package.
- `internal/cli/application.go` is the only production location expected to import many command packages.
- Ask remains an orchestrator without becoming a second composition root.

### Phase 3: make source policy and discovery neutral

- [x] Inventory every consumer of `sourcepolicy.Configure`, `ConfigureSources`, `LoadSources`, Search file listing, Extract discovery, and source-scope inspection.
- [x] Reduce `sourcepolicy` to immutable source-scope options, explicit-path bypass reporting, and neutral repository facts; engine-specific projections now point inward from Search and Extract.
- [x] Establish `internal/sourcecatalog` as the neutral owner of gitignore-aware walking, configured ignores, production classification, deterministic listings, and source-selection decisions.
- [x] Move Search parameter projection to Search.
- [x] Move Extract loading projection to Extract.
- [x] Migrate Tree, Init, Verify, Source Inspection, Search, and Extract to consume the neutral catalog rather than importing Search merely to list files.
- [x] Change Directory Metadata summaries to accept selected files or directories from their caller; remove `internal/directorymeta -> search`.
- [x] Ensure `cliruntime.Repository()` exposes neutral source-policy facts rather than `search.Params`, Extract types, or loader behavior.
- [x] Preserve repository configuration digests, explicit-path bypass notices, `.gitignore` behavior, production-only selection, deterministic source counts, and Extract-specific unsupported/generated-file filtering.

Expected result:

```text
cliruntime -> sourcepolicy
sourcecatalog -> sourcepolicy, pathfilter, sourcekind
search -> sourcecatalog
extract -> sourcecatalog
tree/init/verify -> sourcecatalog
directorymeta -X-> search
sourcepolicy -X-> search
sourcepolicy -X-> extract
```

Exit criteria:

- Importing `cliruntime` does not transitively select a Search or Extract workflow.
- Repository scope has one neutral representation and one discovery implementation.
- Search and Extract tests prove equivalent source selection.
- Directory metadata can be read, written, and summarized without importing the Search engine.

### Phase 4: separate domain models from HTTP contracts

This is the most compatibility-sensitive phase and should follow analysis consolidation.

- [x] Classify every production dependency on `api` as one of:
  - transport boundary,
  - serialization requirement,
  - shared domain data,
  - accidental convenience.
- [x] Define canonical domain models in their owning packages: Search owns requests/results and Navigation owns relationship, artifact, and external-resolution models.
- [x] Convert wire-compatible `api.SearchRequest` aliases to Search parameters through the Search-owned request model and validation boundary.
- [x] Return Search-owned results from Search; retain `api.FileResult` as a wire-compatible alias at transport boundaries.
- [x] Move navigation resolution models to Navigation when they are used by algorithms rather than only serialization.
- [x] Change Analysis graph queries to use the analysis-owned `GraphQuery`; CLI transport adapters construct `api.GraphQueryRequest` only for remote requests.
- [x] Normalize Rules through rules-owned rule, structural-request, and limit types; retain API names as wire-compatible aliases.
- [x] Migrate rendering to canonical Search and Navigation result models; retain `internal/render -> api` only for transport envelopes, result metadata, source-analysis summaries, and Tree wire responses.
- [x] Use type aliases and generic adapter constraints to preserve exported library and wire compatibility during migration.
- [x] Defer removal of explicitly supported compatibility aliases to a separate breaking release rather than silently changing public APIs.

Expected result:

```text
api -> search/navigation/analysis domain models
search/navigation/analysis -X-> api
```

Exit criteria:

- Search, Navigation, Analysis, and Rules can be tested without constructing HTTP DTOs.
- JSON and remote API schemas remain compatible or receive an explicit schema/version change.
- Conversion code is concentrated at transport boundaries.

### Phase 5: clarify Parser, Navigation, Search, Extract, and Analysis ownership

Do not split or merge these packages solely because they are large. Use source-backed call and change-impact evidence for each move.

- [x] Document the intended responsibility of each package in `docs/domain-boundaries.md`:
  - Parser: syntax trees, language adapters, declarations, references, and syntax-owned facts.
  - Navigation: graph construction, repository/language resolution, generic filtering, and traversal.
  - Search: text and structural query execution, target selection, ranking, and attaching related navigation evidence to matches.
  - Extract: focused semantic structure/flow projections and Mermaid parsing, validation, and generation.
  - Analysis: aggregate architecture, graph, boundary, responsibility, comparison, statistics, and diff reports over parsed/navigation facts.
  - Source Catalog: source discovery, ignore evaluation, classification, and deterministic selection decisions.
- [x] Inventory `parser/navigation*.go`, `navigation/*`, `search/navigation*.go`, and Extract language/flow adapters in `docs/implementation-ownership-inventory.md`; classify each file group by responsibility.
- [x] Separate Parser-owned syntax facts from Extract-owned diagram semantics; retain direct `parser.ViewNode` interpretation only in terminal language adapters where it produces diagram-specific facts.
- [x] Move generic graph filters, queries, resolution statistics, and semantic diffs from Search to Navigation; retain Search aliases only for exported compatibility.
- [x] Keep Search-specific related-result budgeting, preview expansion, omission accounting, and result projection in Search.
- [x] Move graph-backed boundary heuristics to the Analysis-owned `internal/boundaryanalysis` implementation and expose them through Analysis facades.
- [x] Migrate internal graph consumers from pure Search-to-Navigation forwarding wrappers; retain exported aliases until an explicitly breaking release.
- [x] Reduce exported implementation surfaces: canonical behavior is behind focused facades; remaining exported domain models and compatibility aliases are intentional contracts.
- [x] Use internal implementation subpackages only for cohesive subsystems that break a real dependency (`internal/boundaryanalysis`) or isolate generated code.
- [x] Avoid bidirectional Parser/Navigation dependencies; Parser remains foundational and architecture evidence shows no Parser-to-Navigation edge.
- [x] Keep language-specific syntax knowledge in Parser navigation adapters or terminal Extract language adapters; Navigation consumes Parser facts rather than duplicating syntax parsing.

Exit criteria:

- Each navigation, search, extraction, and analysis stage has one documented owner.
- Extract does not own generic source discovery or duplicate Parser facts without an explicit semantic reason.
- Package APIs describe capabilities rather than exposing implementation structures.
- No new dependency cycles are introduced.
### Phase 6: move process configuration to the composition boundary

- [x] Remove Parser cache environment configuration from `cmd/grepple/main.go`.
- [x] Keep the CLI-specific cache default in an explicit `internal/cli` process bootstrap; Parser remains library-neutral.
- [x] Move Ask/Init model-preference loading to `internal/usersettings` and provider/model precedence to `internal/aiprovider`; command prompts and tools remain separate.
- [x] Keep `cmd/grepple` limited to signal handling, invoking the CLI, mapping exit codes, and printing terminal errors.
- [x] Avoid hidden mutable globals where invocation scope is practical: repository options, spill thresholds, and exit state now flow through each command context; remaining globals are immutable tables, guarded caches, build metadata, or instrumentation counters.
- [x] Preserve documented environment overrides, explicit-empty cache disabling, and shared Ask/Init model-selection parity.
Exit criteria:

```text
cmd/grepple -> internal/cli
cmd/grepple -X-> parser
```

### Phase 7: remove compatibility wrappers, dead facades, and speculative packages

- [x] Inventory every package-level `Run` wrapper; only the root `internal/cli.Run` protocol entrypoint and test-local helpers remain.
- [x] Remove command-package compatibility wrappers because no supported production or embedding consumer requires them.
- [x] Migrate internal command callers and tests to command objects (`New(context).Run(args)`) or private implementation seams.
- [x] Remove dead adapters, duplicate DTOs, and obsolete parity helpers; retain only documented Search/API compatibility aliases until a breaking release.
- [x] Merge `internal/agent/research` into Ask because no second consumer exists and the prompt is Ask-specific policy.
- [x] Make `internal/filedigest` the canonical file-digest owner and migrate Directory Metadata to it while preserving prefixed source-policy digests and raw metadata checksums.
- [x] Retain `internal/outputspill` as a cohesive CLI-owned subsystem with focused package tests and an explicit ownership rationale in `docs/domain-boundaries.md`.
- [x] Retain `dependency`, `rulespec`, generated grammar packages, hook internals, and command adapters for the domain, compatibility, generation, or terminal-adapter reasons recorded in `docs/domain-boundaries.md`.
- [x] Do not merge a package solely because it has one importer; use the documented cohesion, compatibility, testing, generated-code isolation, and dependency-containment criteria.
- [x] Keep composition tests in `internal/cli`; keep command and subsystem behavior tests with their implementation owners.

Exit criteria:

- Production command registration uses command objects consistently.
- No facade exists solely to preserve an internal dependency that has already been removed.
- Every remaining one-consumer package has an explicit ownership rationale or external compatibility obligation.
- Public removals are documented as compatibility decisions.

### Phase 8: enforce the final architecture

- [x] Regenerate the production-only Mermaid graph after each completed boundary migration.
- [x] Compare before and after production relations and explain every intentional new edge in `docs/restructure-relations.md`.
- [x] Add `internal/cli/import_policy_test.go` checks for:
  - sibling command imports,
  - `cliruntime -> render`,
  - reusable engine -> `internal/cli`,
  - core model -> `api` after transport separation.
- [x] Run focused package tests after each phase and `go test ./...` at every merge boundary.
- [x] Run `git diff --check` at every completed migration boundary.
- [x] Update `README.md`, architecture/output documentation, metadata, and command help whenever behavior or schemas change.
- [x] Mark the migration roadmap historical on completion and retain only the ownership and relation documents as current guidance.

## Migration discipline

Each migration should be independently reviewable:

1. Add or expose the canonical lower-level capability.
2. Add parity tests against current behavior.
3. Migrate one consumer.
4. Verify focused and repository-wide tests.
5. Remove the old implementation only after the final consumer moves.
6. Regenerate architecture evidence and verify dependency direction.

Avoid commits that simultaneously move files, rename public types, change output schemas, and alter behavior. Mechanical moves and semantic changes should be separate whenever possible.

## Definition of done

The restructure is complete when:

- [x] Architecture and Graph each have one reusable implementation owner, including Compare and Responsibilities.
- [x] Production command packages do not import sibling command packages.
- [x] `cliruntime.Context` remains command-neutral and does not expose rendering or command-specific models.
- [x] Source policy and source discovery have neutral owners; neither imports Search or Extract, and Directory Metadata does not import Search.
- [x] Core Search, Navigation, Analysis, and Rules behavior does not depend on HTTP DTOs.
- [x] Parser, Navigation, Search, Extract, Analysis, and Source Catalog responsibilities are documented and reflected by source ownership.
- [x] `cmd/grepple` does not configure Parser internals directly.
- [x] Ask and Init share model-selection configuration without sharing command-specific prompts or tools.
- [x] Deprecated wrappers and transitional facades are either removed or explicitly supported.
- [x] Every one-consumer package has a documented reason to remain separate.
- [x] Production-only architecture output has no unexplained reverse or lateral dependency edges.
- [x] Local and remote reports remain semantically equivalent.
- [x] Exact explicit ranges, source counts, output limits, spill behavior, and CLI exit behavior retain regression coverage.
- [x] `go test ./...` and `git diff --check` pass.