# CLI command object refactor

## Goal

Represent every CLI command as an object implementing one command-neutral interface. Each command object owns its explicit dependencies and command behavior, while `internal/cli` retains process orchestration, repository invocation scope, output spilling, and application dispatch.

## Current architecture

- `internal/cli/args.go` owns dispatch and invokes package-level `Run(args, Dependencies)` functions.
- Extracted command packages already import `internal/cli/runtime`; production imports remain one-way.
- Most command packages repeat a `Dependencies` bundle, which is a strong signal for a command object.
- Parent adapters bridge process-owned streams, configuration, authentication, repository state, remote transport, caches, and shared analysis services.
- Search remains in the parent package and is both the explicit `search` command and the implicit default.
- Graph has child-owned dispatch, but build, resolve, diff, and query behavior still lives in parent adapters.

## Decisions

### Minimal command contract

Add the contract to `internal/cli/runtime`:

```go
type Command interface {
    Run(args []string) error
}

type CommandFunc func(args []string) error
```

`CommandFunc` permits behavior-preserving adapters during incremental migration. Do not add names, aliases, help, or availability to the interface; application registration owns that metadata.

A future cancellation-oriented change may add `context.Context`, but this refactor preserves the current signature.

### Command construction

Each command package should expose `New(Dependencies) runtime.Command` and keep its implementation private:

```go
type command struct {
    dependencies Dependencies
}

func New(dependencies Dependencies) runtime.Command {
    return &command{dependencies: dependencies}
}

func (c *command) Run(args []string) error {
    // Parse and execute command-owned behavior.
}
```

Use `rules.New`, not `NewRulesCommand`, because the package name already supplies the command identity.

### Configuration and services

Do not inject one broad shared `*Config` into every command. Keep these concerns distinct:

1. Immutable configuration values such as server defaults and output limits.
2. Runtime services such as authenticated requests, source loading, Git inspection, caches, and remote analysis.
3. Process streams: stdin, stdout, and stderr.

Dependencies should remain narrow and command-specific. Store `io.Writer` rather than a reusable `*runtime.Output`; create mutable output state per execution.

### Application registry

The parent package owns an invocation-scoped application registry:

```go
type commandSpec struct {
    name         string
    aliases      []string
    command      runtime.Command
    availability commandAvailability
}

type application struct {
    commands       map[string]commandSpec
    defaultCommand runtime.Command
}
```

Construct a fresh application for each top-level invocation so tests and embedded users can change streams, environment, settings, and repository state without stale global state.

Search is registered under `search` and as `defaultCommand`. A `grep` alias should only be added as an explicit user-facing compatibility decision.

## Migration plan

### Phase 1: command contract and application registry

- [x] Add `runtime.Command` and `runtime.CommandFunc` with unit tests.
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
- [x] get
- [x] tree
- [x] write

For each package, add an unexported `command`, add `New`, move `Run` to a method, update registration, and colocate command tests. Deprecated package-level wrappers remain temporarily for test and embedded-call compatibility; remove them after all consumers migrate and before the completion criteria are closed.

### Phase 3: behavior-rich extracted commands

- [x] rules
- [x] extract
- [x] grit

Subcommand behavior is attached to command receivers (`runAdd`, `runList`, `runResults`, focused extraction operations, and local/remote/explain GritQL execution). Deprecated package-level wrappers remain only as temporary compatibility entrypoints.

### Phase 4: complete graph ownership

- [ ] Move graph build, resolve, diff, and query orchestration onto one graph command object.
- [ ] Inject narrow graph-building, remote-analysis, repository, and output services.
- [ ] Leave reusable navigation and analysis primitives below the command package.
- [ ] Remove parent graph behavior adapters after all consumers use reusable APIs.

Progress: graph dispatch is now an invocation-scoped `runtime.Command` constructed through `graph.New`. The graph package owns the normalized projection model, supported-source selection, graph assembly, and source accounting used by commands, boundaries, research, and parity tests through parent compatibility aliases. Symbol resolution now also owns its argument parsing, filtering, metadata, continuation commands, rendering, and exit behavior in `internal/cli/graph`; the parent supplies only output, graph loading, repository flags, and exit state. Build, diff, and query parsing/rendering callbacks remain parent-owned.

### Phase 5: architecture and boundaries

- [x] Convert command entrypoints to objects.
- [x] Keep reusable report construction and analysis APIs independent of command execution.

### Phase 6: search

- [ ] Create `internal/cli/search` after its renderer, context, repository, anchor, and transport boundaries are stable.
- [ ] Make one search command instance serve explicit `search` dispatch and implicit default dispatch.
- [ ] Keep output spilling and repository invocation scope at the application layer.
- [ ] Decide separately whether `grep` becomes a supported alias.

### Phase 7: settings and authentication families

- [ ] Convert anchors after introducing narrow settings/provider services.
- [ ] Convert ai-provider, login, and logout together around shared credential and provider state.
- [ ] Avoid a broad settings service locator.

## Testing and architecture gates

For every phase:

- Test commands through `New(...).Run(...)`.
- Keep parent tests focused on registration, aliases, default search, repository scope, spilling, and process exit behavior.
- Verify child command packages never import parent `internal/cli`.
- Preserve existing help and error text unless intentionally changed.
- Run full tests, race tests, lint, vet, generated/schema checks, native build, and `git diff --check`.

## Completion criteria

- Every CLI command implements `runtime.Command`.
- Constructors receive explicit narrow dependencies.
- The dispatcher no longer contains a command switch.
- Package-level command entrypoints and compatibility wrappers are removed.
- Search is both registered explicitly and configured as the application default.
- Parent adapters contain only application/process infrastructure.
- Command behavior tests are colocated with their owning package.
- No child command package imports parent `internal/cli`.