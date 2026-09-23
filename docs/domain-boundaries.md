# Domain and transport boundaries

Core packages own behavior and canonical models. The `api` package owns HTTP wire compatibility: it aliases Search and Navigation domain models where schemas are identical and owns transport-only envelopes and metadata. CLI and HTTP adapters apply transport defaults and translate where representations differ; rendering may consume API responses when it is explicitly rendering transport output.

## Package responsibilities

- **Parser** owns syntax trees, language adapters, declarations, references, and syntax-derived facts.
- **Navigation** owns graph construction, repository and language resolution, generic graph filtering and traversal, semantic graph diffs, relationship models, and external dependency resolution models.
- **Search** owns text and structural query execution, target selection, ranking, request validation/defaulting, canonical search results, and related-evidence attachment to search matches.
- **Extract** owns focused semantic structure and flow projections plus Mermaid parsing, validation, and generation. Source discovery belongs to Sources.
- **Analysis** owns aggregate architecture, graph, boundary, responsibility, comparison, statistics, and diff reports over parser and navigation facts.
- **Sources** (`internal/sources`) owns neutral repository policy, source walking, ignore evaluation, metadata-backed classification, deterministic listing, source-selection decisions, and source-scope inspection reports.
- **Rules** owns normalized saved-rule semantics, validation, rule models, and structural rule limits; API rule objects are wire-compatible aliases.

## Remaining API dependencies

| Dependency | Current classification | Intended boundary |
| --- | --- | --- |
| `api/model.go -> search` | Wire-compatible aliases preserve existing `api.SearchRequest`, `api.FileResult`, and related result names | Keep aliases until an explicitly breaking release; transport-only response envelopes remain in API. |
| `api/model.go -> navigation` | Wire-compatible aliases preserve navigation artifact, relationship, and resolution names | Keep aliases until an explicitly breaking release; algorithms consume Navigation-owned models. |
| `api/model.go`, `api/grit.go -> rulespec` | Wire-compatible aliases preserve rule and structural-request transport names | Keep aliases until an explicitly breaking release. |
| `internal/render -> api` | Intentional serialization dependency limited to transport envelopes, metadata, source-analysis summaries, and Tree responses | Search and Navigation result rendering uses canonical domain models. |

Analysis graph traversal, Search request/result construction, Navigation external dependency resolution, and Rules normalization no longer depend on API DTO declarations. Local callers use domain models; API aliases preserve source and JSON compatibility, while remote adapters retain transport-only envelopes and metadata. These classifications are migration guidance, not permission to add new core-to-API dependencies. The final relation changes are recorded in [Restructure relation review](restructure-relations.md).

## Cohesive single-consumer packages

`internal/outputspill` remains a separate CLI-owned subsystem even though `internal/cli` is its only production consumer. It encapsulates process-wide stdout capture, immutable artifact persistence, digest naming, descriptor serialization, and spill delivery behind one tested boundary; the application-level `internal/cli.Arguments` tree owns parsing spill controls together with every other invocation argument. The legacy `outputspill.Parse` helper remains only as a tested compatibility API.

Other low-import-count packages are retained for concrete boundaries rather than importer count: `dependency` owns independently tested manifest and lockfile resolution; `rulespec` owns saved-rule compatibility and normalization; generated grammar packages isolate generated code from handwritten adapters; hook internals isolate installation-time behavior from the CLI; and command packages terminate dependency flow by exporting command-specific argument models and executing already-parsed values against `cliruntime.Context`. Importer count alone is not a merge criterion—domain cohesion, public compatibility, independent testing, generated-code isolation, and dependency containment govern these decisions.

## CLI composition and parsing

`internal/cli.Arguments` is the single production `go-arg` tree. It owns top-level commands, nested subcommands, global repository/output controls, default-search normalization, and dispatch after the complete invocation has parsed successfully. Command packages export their argument structs and typed `Execute` entrypoints; they retain command-local semantic validation and execution, but the production composition path does not hand unparsed argument slices between commands. `cliruntime.Context` remains argument-neutral and carries only invocation services and state.