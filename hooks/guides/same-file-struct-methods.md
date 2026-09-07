# same-file-struct-methods: keep a struct's methods in its own file

## Goal

Every method of a struct lives in the file that declares the struct, so a
type's whole surface — fields plus behavior — is readable in one place and
`grepple -l` / file outlines tell the full story without cross-file hunts.

## Preferred approach

1. The diagnostic names the target file (the one holding the `type` decl).
2. Move the method there — cut the `func (s *T) Name(…) {…}` block, paste it
   near its siblings, keep source order sensible (constructors first, then
   methods in reading order).
3. Move any method-local helpers, constants, or types only if the moved method
   is their sole user; otherwise leave them and let the moved method reference
   them where they are.
4. Rebuild (`go build ./...`) and run the package tests
   (`go test ./internal/<pkg>/`).

## Not acceptable

- Moving the type declaration to a method's file instead (the type is the
  anchor; the diagnostic points the other way).
- Splitting one method into wrapper + impl across files to satisfy the letter
  of the rule.
- Renaming, reordering unrelated code, or refactoring while moving — the diff
  should be a pure relocation.
- Touching `_test.go` placement conventions not flagged by the diagnostic.

## Completion check

`make hook-test` (or the next Stop) no longer reports the moved method,
`go build ./...` passes, and package tests are green.
