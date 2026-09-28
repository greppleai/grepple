# `internal/navigation` exported standalone-function hook audit

- Date: 2026-09-28
- Baseline revision: `706dd40` (before the navigation service-interface refactor).
- Requested hook: `go-standalone-methods` (not defined); executed `go-standalone-functions`. This rule checks exported package-level functions; methods are exempt.
- Command: `go run ./cmd/grepple hook --all --id go-standalone-functions --json`
- Baseline scope: full repository relational scan (1008 selected files), then results filtered to `internal/navigation/`; `--all` overrides `report_changed_only`.
- Baseline results: **14 warnings across 6 files** in `internal/navigation/` (247 findings repository-wide); **0 reported scan/configuration failures**. Exit status 1 means findings were present, not a failed scan.
- Compared with the earlier unfiltered audit: 117 navigation warnings became 14 after restricting the hook to exported function names. `navigationFieldKey` was no longer flagged; `BuildGraphFromDocuments` was flagged in this baseline.

The rule matches package-level Go functions whose names start with a Unicode uppercase letter and whose declared results include no inline, predeclared, or same-package named interface. Methods, `init`, `main`, tests, vendor files, and testdata are exempt. It does not resolve imported interfaces, aliases, or build tags, and **a warning is not a requirement to refactor**. Every finding below carries the hook message: “Review this exported package-level function: no declared result is a known interface in its package. Methods are exempt; justified stateless helpers may remain.”

## Baseline findings (historical revision `706dd40`)

### `internal/navigation/diff.go` (1 warning)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 39 | [`DiffNavigationGraphs`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/diff.go#L39) | warning |

### `internal/navigation/engine.go` (7 warnings)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 137 | [`BuildGraph`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/engine.go#L137) | warning |
| 143 | [`BuildGraphWithStats`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/engine.go#L143) | warning |
| 148 | [`BuildGraphWithOptions`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/engine.go#L148) | warning |
| 154 | [`BuildAnalysisFromDocuments`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/engine.go#L154) | warning |
| 182 | [`BuildAnalysisFromTextSources`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/engine.go#L182) | warning |
| 205 | [`BuildGraphFromDocuments`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/engine.go#L205) | warning |
| 211 | [`BuildGraphFromTextSources`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/engine.go#L211) | warning |

### `internal/navigation/external_dependencies.go` (2 warnings)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 124 | [`ExternalDependencyReferences`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/external_dependencies.go#L124) | warning |
| 164 | [`ApplyExternalDependencyResolution`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/external_dependencies.go#L164) | warning |

### `internal/navigation/external_manifest_dependencies.go` (2 warnings)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 13 | [`NormalizeNPMRegistryEvidence`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/external_manifest_dependencies.go#L13) | warning |
| 18 | [`ValidNPMIntegrity`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/external_manifest_dependencies.go#L18) | warning |

### `internal/navigation/navigation_go_repository.go` (1 warning)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 86 | [`RepositoryContextFiles`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/navigation_go_repository.go#L86) | warning |

### `internal/navigation/stats.go` (1 warning)

| Line | Exported package-level function | Severity |
| ---: | --- | --- |
| 60 | [`MeasureNavigationResolution`](https://github.com/greppleai/grepple/blob/706dd40/internal/navigation/stats.go#L60) | warning |

## After the navigation, parser, and Rust-module ownership refactors (working tree)

A fresh `go run ./cmd/grepple hook --all --id go-standalone-functions --json` scanned 1015 selected files. It reported **zero findings in the `internal/navigation` package root** and five in its `rustmodule` subpackage; 11 remain in `internal/parser`. The remaining 199 warnings are elsewhere (215 repository-wide). The nonzero exit status is due to those warnings; no source, scan, or configuration failure was reported. The baseline table above is retained for review of the pre-refactor navigation package surface; its function names refer to revision `706dd40` and may now be private methods or implementation helpers.
