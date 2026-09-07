# package-comments: one package doc comment per package

## Goal

Each package has exactly one doc comment (`// Package foo …`) on one file's
`package` clause, so `go doc` shows a coherent package overview.

## Preferred approach

1. Put the comment on the file that best represents the package (often the
   file named after the package, or `doc.go` for large packages).
2. One or two sentences: what the package provides, not how.
   `// Package search implements the local matching engine …`.
3. If the package already has the comment on another file, remove the duplicate
   from the reported file rather than adding a second one.

## Not acceptable

- Empty comments (`// Package foo.`) that say nothing.
- Duplicating the comment across several files.
- Moving the package clause to satisfy the linter.

## Completion check

`revive -config revive.toml ./...` reports no `package-comments` diagnostic
and `go doc ./internal/<pkg>` shows the overview.
