# Parser, Navigation, Search, Extract, and Analysis ownership inventory

This inventory records the post-restructure owner of previously overlapping implementation areas. Compatibility aliases are not implementation owners.

## Parser

`parser/navigation.go`, `navigation_adapter.go`, `navigation_context.go`, `navigation_rust_import.go`, and navigation cache files own syntax-derived declarations, references, imports, visibility, source positions, parsed-document adaptation, and reusable cached syntax facts. Language-specific syntax remains in the parser package. The parser package does not import Navigation.

`parser/service.go` exposes the `Parser` interface for language discovery and capabilities, a `GetGrammar(language)` metadata handle, parsing one caller-owned document, and deriving its navigation facts, outline, and structural segments without reparsing. `parser/grammar_service.go` owns the focused read-only grammar queries. The concrete `Document` retains its explicit `Close` lifecycle and owns whole-document named-node walks; callback-scoped `Read` and `ViewNode.WalkNamed` support coherent subtree traversal. Cross-file Rust module resolution and non-document format helpers remain separate concerns.

## Navigation

`navigation/engine.go` and `navigation/navigation_*.go` own repository-wide declaration and call resolution over Parser facts. `filter.go`, `query.go`, `stats.go`, and `diff.go` own generic graph filtering, traversal, statistics, and semantic diffs. `model.go` and `external_dependencies.go` own relationship, artifact, and external dependency resolution models and algorithms. Navigation depends inward on Parser; the reverse edge is forbidden.

`navigation/rustmodule` owns selected-crate module indexing, cross-file Rust import/ownership resolution, visibility checks, and crate-root selection over parser-emitted facts. Both Navigation and Extract consume it without routing Rust module resolution through `parser.Parser`. Parser separately identifies executable Rust `main` entrypoints from a single source path; it does not import Navigation.

`navigation/services.go` exposes graph construction and read-only graph operations as narrow interfaces returned by constructors; `external_services.go` exposes generic dependency qualification and resolution capabilities without losing result types. Implementation functions stay package-private. `api.IndexedNavigation` remains the separate backend-facing opaque artifact facade, and `dependency` owns manifest and npm validation directly.

`navigation/navigation_language_index.go` registers each supported language family behind one `languageNavigationIndex`, embedding the focused `languageImportResolver` contract. A typed import request carries source, scope, imported name, and kind; each family owns its repository-specific lookup and returns selected file or module-scope targets. Rust crate modules and ECMAScript `tsconfig.json` aliases use this same route, so new language configuration belongs in the family resolver rather than Parser or the shared graph engine.

## Search

`search/navigation_projection.go` and related-search code own match-centered projection, related-result budgets, preview expansion, omission accounting, ranking, and Search result conversion. `search/navigation_{filter,query,stats,diff}.go` and `boundary_compat.go` are explicitly supported source-compatibility aliases, not implementations.

## Extract

`extract/language.go`, language adapters, `focused_language_semantics.go`, `flow.go`, and focused/module flow files own diagram-specific declarations, members, flow nodes, Mermaid-safe types, and focused semantic projections. Their `parser.ViewNode` interpretation is retained only where the fact is diagram-semantic or language-specific. Generic source discovery and graph traversal are not implemented here. Shared syntax locations, node text, declarations, references, imports, calls, visibility, and navigation facts come from Parser APIs.

The remaining direct `ViewNode` helpers are deliberately adapter-local: they interpret language grammar into Mermaid member/type/flow semantics. Moving them to Parser would make Parser depend on diagram concepts without creating a second consumer. Repeated generic walking and source-location mechanics already use Parser's `Document.WalkNamedView`, `ViewNode.WalkNamed`, and navigation-fact APIs.

## Analysis

`analysis` owns public architecture, graph, responsibility, comparison, and boundary report facades. `internal/boundaryanalysis` is its cohesive implementation subsystem for graph-backed workflow, type-spread, policy, and facade-bypass heuristics. Search retains aliases only for exported compatibility. CLI boundary rendering consumes Analysis models.

## Sources

`internal/sources` is the cohesive source-universe boundary. It owns repository policy options, gitignore/configured-ignore discovery, metadata-backed file kinds, production-only filtering, explicit bypass notices, deterministic selection decisions, and source-scope reports. Missing or stale `.grepple/grepple.yaml` classifications are `unknown`; path names do not infer source purpose. `internal/sourcelocation` separately owns only `PATH:LINE[-END]` syntax and is intentionally not part of source discovery.

## CLI application

`internal/cli/application.go` owns the complete `go-arg` command tree and typed dispatch. Top-level process controls and every command/subcommand are parsed before execution. `internal/cli/<command>` packages own exported argument models, semantic validation, and execution of parsed values; they do not import sibling commands. `internal/cliruntime` remains the command-neutral invocation service boundary.

## Boundary rules

- Implementation subpackages are used only for cohesive subsystems that break a dependency or isolate generated code.
- Parser remains foundational; no Parser-to-Navigation edge is allowed.
- Search-specific related projection remains in Search.
- Language-specific syntax stays in Parser or terminal Extract language adapters; generic graph behavior stays in Navigation or Analysis.