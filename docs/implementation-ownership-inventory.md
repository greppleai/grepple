# Parser, Navigation, Search, Extract, and Analysis ownership inventory

This inventory records the post-restructure owner of previously overlapping implementation areas. Compatibility aliases are not implementation owners.

## Parser

`parser/navigation.go`, `navigation_adapter.go`, `navigation_context.go`, `navigation_rust_import.go`, and navigation cache files own syntax-derived declarations, references, imports, visibility, source positions, parsed-document adaptation, and reusable cached syntax facts. Language-specific syntax remains in Parser. Parser does not import Navigation.

## Navigation

`navigation/engine.go` and `navigation/navigation_*.go` own repository-wide declaration and call resolution over Parser facts. `filter.go`, `query.go`, `stats.go`, and `diff.go` own generic graph filtering, traversal, statistics, and semantic diffs. `model.go` and `external_dependencies.go` own relationship, artifact, and external dependency resolution models and algorithms. Navigation depends inward on Parser; the reverse edge is forbidden.

## Search

`search/navigation_projection.go` and related-search code own match-centered projection, related-result budgets, preview expansion, omission accounting, ranking, and Search result conversion. `search/navigation_{filter,query,stats,diff}.go` and `boundary_compat.go` are explicitly supported source-compatibility aliases, not implementations.

## Extract

`extract/language.go`, language adapters, `focused_language_semantics.go`, `flow.go`, and focused/module flow files own diagram-specific declarations, members, flow nodes, Mermaid-safe types, and focused semantic projections. Their `parser.ViewNode` interpretation is retained only where the fact is diagram-semantic or language-specific. Generic source discovery and graph traversal are not implemented here. Shared syntax locations, node text, declarations, references, imports, calls, visibility, and navigation facts come from Parser APIs.

The remaining direct `ViewNode` helpers are deliberately adapter-local: they interpret language grammar into Mermaid member/type/flow semantics. Moving them to Parser would make Parser depend on diagram concepts without creating a second consumer. Repeated generic walking and source-location mechanics already use Parser's `ViewNode`, `WalkNamedView`, document, and navigation-fact APIs.

## Analysis

`analysis` owns public architecture, graph, responsibility, comparison, and boundary report facades. `internal/boundaryanalysis` is its cohesive implementation subsystem for graph-backed workflow, type-spread, policy, and facade-bypass heuristics. Search retains aliases only for exported compatibility. CLI boundary rendering consumes Analysis models.

## Sources

`internal/sources` is the cohesive source-universe boundary. It owns repository policy options, gitignore/configured-ignore discovery, metadata-backed file kinds, production-only filtering, explicit bypass notices, deterministic selection decisions, and source-scope reports. Missing or stale `grepple.yaml` classifications are `unknown`; path names do not infer source purpose. `internal/sourcelocation` separately owns only `PATH:LINE[-END]` syntax and is intentionally not part of source discovery.

## CLI application

`internal/cli/application.go` owns the complete `go-arg` command tree and typed dispatch. Top-level process controls and every command/subcommand are parsed before execution. `internal/cli/<command>` packages own exported argument models, semantic validation, and execution of parsed values; they do not import sibling commands. `internal/cliruntime` remains the command-neutral invocation service boundary.

## Boundary rules

- Implementation subpackages are used only for cohesive subsystems that break a dependency or isolate generated code.
- Parser remains foundational; no Parser-to-Navigation edge is allowed.
- Search-specific related projection remains in Search.
- Language-specific syntax stays in Parser or terminal Extract language adapters; generic graph behavior stays in Navigation or Analysis.