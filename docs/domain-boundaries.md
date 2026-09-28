# Domain and transport boundaries

Core packages own behavior and canonical models. `internal/wire` owns HTTP transport envelopes and metadata used by CLI and HTTP adapters; `api` re-exports wire contracts and provides the only supported external Go service facade. Compatibility aliases in wire still refer to Search, Navigation, and Rulespec models where schemas are identical. Internal packages must not import `api`, so services can depend inward on implementations without a reverse dependency. Execution handles such as `api.IndexedNavigation` and `api.SearchPlan` are interfaces with private implementations; concrete options and request/response DTOs carry values across the boundary.

## Package responsibilities

- **Parser** owns syntax trees, language adapters, declarations, references, and syntax-derived facts.
- **Navigation** owns graph construction, repository and language resolution, generic graph filtering and traversal, semantic graph diffs, relationship models, and external dependency resolution models.
- **Search** owns text and structural query execution, target selection, ranking, request validation/defaulting, canonical search results, and related-evidence attachment to search matches.
- **Extract** owns focused semantic structure and flow projections plus Mermaid parsing, validation, and generation. Source discovery belongs to Sources.
- **Analysis** owns aggregate architecture, graph, boundary, responsibility, comparison, statistics, and diff reports over parser and navigation facts.
- **Sources** (`internal/sources`) owns neutral repository policy, source walking, ignore evaluation, metadata-backed classification, deterministic listing, source-selection decisions, and source-scope inspection reports.
- **Rules** owns normalized saved-rule semantics, validation, rule models, and structural rule limits; API rule objects are wire-compatible aliases.

## Dependency direction and compatibility

| Dependency | Purpose | Boundary |
| --- | --- | --- |
| `api -> internal/wire` | Re-export stable HTTP DTOs and constants | Only the external `api` facade publishes these contracts to backend consumers. |
| `api -> internal/analysis,search,navigation,...` | Backend-facing services | Services call canonical implementations; implementations never call back into `api`. |
| `internal/cli,apiclient,render,gritqlapi -> internal/wire` | Construct, decode, and render transport results | Internal adapters consume shared DTOs directly rather than importing the public facade. |
| `internal/wire -> internal/search,navigation,rulespec` | Preserve existing type identities for compatible wire aliases | These aliases are compatibility debt; new behavior belongs to engine owners and new API services should prefer opaque or facade-owned representations. |

Analysis graph traversal, Search request/result construction, Navigation external dependency resolution, and Rules normalization do not depend on public API DTO declarations. A source-level guard checks every implementation and test import for `api`; backend code has a complementary guard allowing only `api` from this module.

## Cohesive single-consumer packages

`internal/outputspill` remains a separate CLI-owned subsystem even though `internal/cli` is its only production consumer. It encapsulates process-wide stdout capture, immutable artifact persistence, digest naming, descriptor serialization, and spill delivery behind one tested boundary; the application-level `internal/cli.Arguments` tree owns parsing spill controls together with every other invocation argument. The legacy `outputspill.Parse` helper remains only as a tested compatibility API.

Other low-import-count packages are retained for concrete boundaries rather than importer count: `dependency` owns independently tested manifest and lockfile resolution; `rulespec` owns saved-rule compatibility and normalization; generated grammar packages isolate generated code from handwritten adapters; hook internals isolate installation-time behavior from the CLI; and command packages terminate dependency flow by exporting command-specific argument models and executing already-parsed values against `cliruntime.Context`. Importer count alone is not a merge criterion—domain cohesion, public compatibility, independent testing, generated-code isolation, and dependency containment govern these decisions.

## CLI composition and parsing

`internal/cli.Arguments` is the single production `go-arg` tree. It owns top-level commands, nested subcommands, global repository/output controls, default-search normalization, and dispatch after the complete invocation has parsed successfully. Command packages export their argument structs and typed `Execute` entrypoints; they retain command-local semantic validation and execution, but the production composition path does not hand unparsed argument slices between commands. `cliruntime.Context` remains argument-neutral and carries only invocation services and state.