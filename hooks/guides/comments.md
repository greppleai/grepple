# comments: fix all doc-comment issues in this file in one pass

## Goal

Every exported symbol carries a doc comment starting with its exact name, and
the package has exactly one `// Package foo …` comment. These are posted
together because they are one kind of work: write the documentation this file
owes its readers.

## Preferred approach

1. Read the whole file first; the comments should describe a coherent API, so
   write them as a set, not as isolated one-liners.
2. Exported symbols: one or two sentences stating the contract, starting with
   the symbol's name (`// SearchContent scans …`). State behavior, inputs,
   and error semantics — not the implementation.
3. Package comment: put `// Package foo …` on the most representative file
   (or `doc.go` for large packages); remove duplicates elsewhere.
4. If a symbol should not be public, unexport it instead of documenting it —
   but only when nothing outside the package references it (check with
   `grepple "SymbolName" --remote` first).

## Not acceptable

- Comments that restate the signature (`// Foo is the Foo function`).
- Suppressing with `//nolint` or weakening `revive.toml`.
- Unexporting symbols other packages import.

## Completion check

`revive -config revive.toml ./...` reports no `exported` or
`package-comments` diagnostics for this file, and the new comments read as a
contract a caller can rely on.
