# CLI command object refactor

## Goal

Represent every CLI command as an object implementing one command-neutral interface. Each command object owns its explicit dependencies and command behavior, while `internal/cli` retains process orchestration, repository invocation scope, output spilling, and application dispatch.

## Current architecture

- `internal/cli/application.go` constructs a fresh command registry for each invocation; `args.go` retains top-level parsing and help behavior.
- Command packages import command infrastructure from `internal/cliruntime`; production imports remain one-way.
- Command constructors receive the shared command context; command-specific workflows and implementations remain private to the owning package.
- Root `internal/cli` performs application composition directly. It has no production `*_adapter.go` or `*dependencies*.go` files.
- `internal/cli/search` owns parsing, local and remote execution, result metadata, rendering orchestration, and serves both explicit `search` and implicit default dispatch.
- Graph owns build, resolve, diff, query, compact rendering, and reusable in-memory graph operations under `internal/cli/graph`.

## Decisions

### Minimal command contract

The contract lives in `internal/cliruntime`:

```go
type Command interface {
    Run(args []string) error
}

type Context interface {
    Stdin() io.Reader
    Stdout() io.Writer
    Stderr() io.Writer
    APIClient() apiclient.APIClient
    Configuration() Configuration
    Repository() Repository
    RequestExit(int)
}

type CommandFunc func(args []string) error
```

`CommandFunc` remains a runtime convenience for tests and genuinely function-shaped embedded callers; production command registration uses command objects constructed from `cliruntime.Context`.

A future cancellation-oriented change may add `context.Context`, but this refactor preserves the current signature.

### Command construction

Each command package should expose `New(cliruntime.Context) cliruntime.Command` and keep its implementation private:

type command struct {
    application cliruntime.Context
}

func New(application cliruntime.Context) cliruntime.Command {
    return &command{application: application}
}

func (c *command) Run(args []string) error {
    // Parse and execute command-owned behavior.
}
```

Use `rules.New`, not `NewRulesCommand`, because the package name already supplies the command identity.

### Configuration and services

Every command receives the same command-neutral command context. The context is the boundary for infrastructure assembled by the application layer:

1. Immutable configuration such as server defaults and output limits.
2. The shared API client.
3. Repository invocation context, including the working directory, current repository, source policy, and scope flags.
4. Process streams and exit signaling.

`internal/cliruntime` owns both the command protocol and the concrete invocation environment: streams, API access, immutable configuration, repository identity, repository scope options, global flags, and exit signaling. `internal/sources` projects neutral scope options into source-loader parameters. Commands own their rendering and command-specific workflows.

Command-specific workflows and implementations remain private to their owning package. They are constructed from the shared context rather than exposed to the application composition root; for example, tree owns local tree discovery and graph owns graph build/query orchestration. Mutable output state is still created per execution.

### Application registry

The parent package owns an invocation-scoped application registry:

```go
type commandSpec struct {
    name         string
    aliases      []string
    command      cliruntime.Command
    availability commandAvailability
}

type application struct {
    commands       map[string]commandSpec
    defaultCommand cliruntime.Command
}
```

Construct a fresh application for each top-level invocation so tests and embedded users can change streams, environment, settings, and repository state without stale global state.

Search is registered under `search` and as `defaultCommand`. A `grep` alias should only be added as an explicit user-facing compatibility decision.

## Migration plan

### Phase 1: command contract and application registry

- [x] Add `cliruntime.Command` and `cliruntime.CommandFunc` with unit tests.
- [x] Add an invocation-scoped application registry.
- [x] Register existing command functions through `CommandFunc` without changing behavior.
- [x] Preserve help, availability checks, repository scope, output spilling, version handling, and exit-state conversion.
- [x] Add dispatcher parity tests for named and default commands.

### Phase 2: simple extracted commands

Convert, in small independently buildable commits:

- [x] version
- [x] languages
- [x] examples
- [x] context
- [x] artifacts
- [x] repos
- [x] refs
- [x] get — owns remote retrieval, range outcomes, error reporting, outline rendering, and exit signaling through `cliruntime.Context`.
- [x] sources — owns repository configuration projection, source-scope reporting, rendering, and streams through `cliruntime.Context`.
- [x] tree
- [x] write — owns input decoding, transactional mutation, response emission, exit signaling, and write-coverage projection through `cliruntime.Context`.

For each package, add an unexported `command`, add `New`, move `Run` to a method, update registration, and colocate command tests. Deprecated package-level wrappers remain temporarily for test and embedded-call compatibility; remove them after all consumers migrate and before the completion criteria are closed.

### Phase 3: behavior-rich extracted commands

- [x] rules
- [x] extract
- [x] grit — owns local/remote structural execution, metadata, continuation commands, rendering, and exit signaling through `cliruntime.Context`.
- [x] ask — owns provider selection, research-session construction, local/remote research tools, caching, rendering, and process interaction through `cliruntime.Context`.

Subcommand behavior is attached to command receivers (`runAdd`, `runList`, `runResults`, focused extraction operations, and local/remote/explain GritQL execution). Deprecated package-level wrappers remain only as temporary compatibility entrypoints.

### Phase 4: complete graph ownership

- [x] Move graph build, resolve, diff, and query orchestration into the graph command package.
- [x] Construct graph from the shared command context while keeping graph-specific loading, transport, metadata, and rendering private.
- [x] Leave reusable navigation and analysis primitives below the command package.
- [x] Remove parent graph behavior adapters after all consumers use reusable APIs.

Progress: `internal/cli/graph` owns graph argument parsing, local and remote orchestration, projection, build, resolve, diff, query, metadata, continuation commands, exit signaling, and rendering. `application.go` now supplies only `cliruntime.Context`; boundaries and research consume reusable graph projections and in-memory operations outside composition wiring.

### Phase 5: architecture and boundaries

- [x] Convert command entrypoints to objects constructed from `cliruntime.Context`.
- [x] Keep reusable report construction and analysis APIs independent of command execution.
- [x] Keep architecture source loading, remote transport, rendering, and exit signaling inside `internal/cli/architecture` rather than the composition root.

### Phase 6: search

- [x] Create `internal/cli/search` after its renderer, context, repository, anchor, and transport boundaries are stable.
- [x] Make one search command instance serve explicit `search` dispatch and implicit default dispatch.
- [x] Keep output spilling and repository invocation scope at the application layer.
- [x] Decide separately whether `grep` becomes a supported alias. Decision: retain `search` and implicit search only; do not add a `grep` alias.

### Search rendering and support boundaries

- [x] Move search result renderer selection and implementations into `internal/render`.
- [x] Move persistent context coverage, deduplication, statistics, and invalidation into `internal/render`.
- [x] Consolidate outline loading, formatting, and emission in `internal/render`.
- [x] Move generic command runtime primitives from `internal/cli/runtime` to `internal/cliruntime`.
- [x] Move structural result summaries into `internal/resultanalysis`.
- [x] Keep `internal/cli` subpackages command-owned; non-command support packages live directly under `internal`.
- [x] Retain only command wiring and observable CLI integration tests in parent `internal/cli`.

Search owns its argument model, local and remote execution, merge/window policy, continuation metadata, anchor preparation, and rendering orchestration. It projects parsed options directly into `render.SearchOptions`; the renderer owns preflight eligibility, output limits, warnings, context coverage, and deduplication. The parent package supplies only `cliruntime.Context`, retains invocation/output-spill lifecycle, and contains observable CLI integration tests rather than search implementation.

### Phase 7: settings and authentication families

- [x] Convert anchors after introducing narrow settings/provider services.
- [x] Convert ai-provider, login, and logout together around `cliruntime.Context`, with credential persistence and provider workflows owned by `internal/cli/auth`.
- [x] Avoid a broad settings service locator.

Authentication commands receive only the shared command context and own provider selection, interactive streams, remote login transport, credential persistence, and logout behavior. The composition root registers the three command objects without credential callbacks or an authentication dependency bundle. Anchors retains its narrow settings-operation dependencies until its own context migration.

## Testing and architecture gates

For every phase:

- Test commands through `New(...).Run(...)`.
- Keep parent tests focused on registration, aliases, default search, repository scope, spilling, and process exit behavior.
- Verify child command packages never import parent `internal/cli`.
- Preserve existing help and error text unless intentionally changed.
- Run full tests, race tests, lint, vet, generated/schema checks, native build, and `git diff --check`.

## Completion criteria

- Every CLI command implements `cliruntime.Command`.
- Constructors receive the shared `cliruntime.Context`; command packages derive narrow private runtimes from it.
- The dispatcher no longer contains a command switch.
- Package-level command entrypoints and compatibility wrappers are removed.
- Search is both registered explicitly and configured as the application default.
- Root CLI production files are application/process composition or behavior awaiting extraction; no forwarding adapter files remain.
- Command behavior tests are colocated with their owning package.
- No child command package imports parent `internal/cli`.