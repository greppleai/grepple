# exported: document (or unexport) exported symbols

## Goal

Every exported symbol (func, type, const, var, method) carries a doc comment
starting with its exact name, so pkg.go.dev renders a usable API surface and
readers know the contract without reading the body.

## Preferred approach

1. Write one or two sentences stating the contract, not the implementation:
   `// SearchContent scans content as if it were the file name and …`.
2. Start the comment with the symbol's name (`// Foo …`, not `// This …`).
3. If the symbol should not be public at all, unexport it instead of
   documenting it — but only when nothing outside the package uses it.
4. For test helpers and obvious cases keep it to one line.

## Not acceptable

- Comments that restate the signature (`// Foo is the Foo function`).
- Suppressing with `//nolint` instead of writing the sentence.
- Unexporting symbols that other packages import (check with
  `grepple "SymbolName" --remote` first).

## Completion check

`revive -config revive.toml ./...` reports no `exported` diagnostic at that
location and the comment reads as a contract a caller can rely on.
