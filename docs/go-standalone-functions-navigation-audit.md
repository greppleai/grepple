# `internal/navigation` exported standalone-function hook audit

- Date: 2026-09-28
- Base revision: `74d5f7a` (export-only hook adjustment is uncommitted).
- Requested hook: `go-standalone-methods` (not defined); executed `go-standalone-functions`. This rule checks exported package-level functions; methods are exempt.
- Command: `go run ./cmd/grepple hook --all --id go-standalone-functions --json`
- Scope: full repository relational scan (1008 selected files), then results filtered to `internal/navigation/`; `--all` overrides `report_changed_only`.
- Results: **14 warnings across 6 files** in `internal/navigation/` (247 findings repository-wide); **0 reported scan/configuration failures**. Exit status 1 means findings were present, not a failed scan.
- Compared with the earlier unfiltered audit: 117 navigation warnings became 14 after restricting the hook to exported function names. `navigationFieldKey` is no longer flagged; `BuildGraphFromDocuments` remains flagged.

The rule matches package-level Go functions whose names start with a Unicode uppercase letter and whose declared results include no inline, predeclared, or same-package named interface. Methods, `init`, `main`, tests, vendor files, and testdata are exempt. It does not resolve imported interfaces, aliases, or build tags, and **a warning is not a requirement to refactor**. Every finding below carries the hook message: “Review this exported package-level function: no declared result is a known interface in its package. Methods are exempt; justified stateless helpers may remain.”

## Findings

### `internal/navigation/diff.go` (1 warning)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 39 | [`DiffNavigationGraphs`](../internal/navigation/diff.go#L39) | warning |

### `internal/navigation/engine.go` (7 warnings)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 137 | [`BuildGraph`](../internal/navigation/engine.go#L137) | warning |
| 143 | [`BuildGraphWithStats`](../internal/navigation/engine.go#L143) | warning |
| 148 | [`BuildGraphWithOptions`](../internal/navigation/engine.go#L148) | warning |
| 154 | [`BuildAnalysisFromDocuments`](../internal/navigation/engine.go#L154) | warning |
| 182 | [`BuildAnalysisFromTextSources`](../internal/navigation/engine.go#L182) | warning |
| 205 | [`BuildGraphFromDocuments`](../internal/navigation/engine.go#L205) | warning |
| 211 | [`BuildGraphFromTextSources`](../internal/navigation/engine.go#L211) | warning |

### `internal/navigation/external_dependencies.go` (2 warnings)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 124 | [`ExternalDependencyReferences`](../internal/navigation/external_dependencies.go#L124) | warning |
| 164 | [`ApplyExternalDependencyResolution`](../internal/navigation/external_dependencies.go#L164) | warning |

### `internal/navigation/external_manifest_dependencies.go` (2 warnings)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 13 | [`NormalizeNPMRegistryEvidence`](../internal/navigation/external_manifest_dependencies.go#L13) | warning |
| 18 | [`ValidNPMIntegrity`](../internal/navigation/external_manifest_dependencies.go#L18) | warning |

### `internal/navigation/navigation_go_repository.go` (1 warning)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 86 | [`RepositoryContextFiles`](../internal/navigation/navigation_go_repository.go#L86) | warning |

### `internal/navigation/stats.go` (1 warning)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 60 | [`MeasureNavigationResolution`](../internal/navigation/stats.go#L60) | warning |
