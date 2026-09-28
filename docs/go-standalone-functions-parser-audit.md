# `internal/parser` exported standalone-function hook audit

- Date: 2026-09-28. This is a snapshot of the uncommitted parser-interface and Rust-module ownership working tree.
- Command: `go run ./cmd/grepple hook --all --id go-standalone-functions --json`
- Scope: full repository scan (1015 selected files), filtered to `internal/parser/`; `--all` overrides `report_changed_only`.
- Results: **11 warnings across 6 parser files**, down from 13 after moving both named-node walkers onto `Document` (19 before grammar queries moved behind `parser.Parser.GetGrammar`; 23 before language discovery moved onto Parser). The scan reported 215 findings repository-wide: 11 in Parser, 5 in `rustmodule`, 0 in the Navigation package root, and 199 elsewhere. The command exits 1 because it reports findings.

The rule reviews exported package-level Go functions without a declared interface result. Methods are exempt. It does not resolve imported interface identities or prove that a function should become a method. Functions returning `error` may pass the rule, so **this is not a complete inventory of exports**. All findings below have severity `warning`, not a required-refactor verdict. Links refer to current working-tree source lines.

## Remaining findings

| File | Count | Exported functions (source lines) |
| --- | ---: | --- |
| `match_range.go` | 2 | [`StructuralLineRanges`](../internal/parser/match_range.go#L29) (29), [`EnclosingLineRanges`](../internal/parser/match_range.go#L37) (37) |
| `navigation.go` | 3 | [`BuildNavigationGraph`](../internal/parser/navigation.go#L188) (188), [`DeclarationRangeAt`](../internal/parser/navigation.go#L231) (231), [`DeclarationRangeAtFromDocument`](../internal/parser/navigation.go#L238) (238) |
| `navigation_cache.go` | 1 | [`NavigationFactDigest`](../internal/parser/navigation_cache.go#L38) (38) |
| `navigation_rust_import.go` | 1 | [`RustScopedPath`](../internal/parser/navigation_rust_import.go#L324) (324) |
| `outline.go` | 2 | [`OutlineFile`](../internal/parser/outline.go#L12) (12), [`OutlineFileDepth`](../internal/parser/outline.go#L19) (19) |
| `segments.go` | 2 | [`BuildSegments`](../internal/parser/segments.go#L50) (50), [`BuildSegmentsWithStatus`](../internal/parser/segments.go#L56) (56) |
| **Total** | **11** | |

The parser-interface migration removed four previously flagged document-backed entrypoints: `NavigationGraphFromDocument`, `CachedNavigationGraphFromDocument`, `OutlineFromDocument`, and `BuildSegmentsFromDocument`. It also retired `ParseDocument`, which was not flagged because its results included `error`. The local-only export pass retired `Navigation` and made the bounded view walker and its option/result types private. Language classification and capability metadata live on `parser.Parser`; `Parser.GetGrammar(language)` returns a focused `Grammar` interface for named nodes, tokens, fields, cardinality, and subtypes. `Document.WalkNamed` and `Document.WalkNamedView` now own whole-document traversal, while `ViewNode.WalkNamed` supports subtrees within an active `Read` callback. Every function still in the warning table has a caller outside `internal/parser` in this checkout; external tests count as callers. The remaining Parser list mixes per-file Rust path normalization, source-only helpers, and cache identities; each needs caller-backed review before any further export change.

## Rust warnings outside Parser after the ownership move

These warnings are **not included** in the 25-parser-warning total. Moving repository-wide Rust module resolution did not itself eliminate its five findings; it moved them to their owning package:

| Package | Warning functions (source lines) |
| --- | --- |
| `internal/navigation/rustmodule` (5) | [`BuildRustModuleIndex`](../internal/navigation/rustmodule/index.go#L32) (32), [`RustModuleTargetModuleKey`](../internal/navigation/rustmodule/index.go#L337) (337), [`RustModuleTargetKey`](../internal/navigation/rustmodule/index.go#L342) (342), [`RustModulesShareCrate`](../internal/navigation/rustmodule/index.go#L379) (379), [`RustItemVisibleFrom`](../internal/navigation/rustmodule/index.go#L387) (387) |
