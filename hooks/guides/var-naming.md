# var-naming: Go naming conventions

## Goal

Identifiers follow Go conventions: camelCase, initialisms in all-caps
(`ID`, `URL`, `HTTP`, `JSON`, `API` — not `Id`, `Url`, `Http`), and no
`snake_case` or `MixedCaps_with_underscores`.

## Preferred approach

1. Rename the identifier to the conventional form: `repoId` → `repoID`,
   `httpClient` stays, `UrlString` → `URLString`.
2. Use the code search to find every reference before renaming:
   `grepple -w "repoId" --files-with-matches` then edit each hit.
3. Keep renames mechanical — same semantics, same scope.

## Not acceptable

- Renaming to something unrelated to dodge the rule.
- Changing exported API names without checking external callers
  (`grepple "OldName" --remote`).
- Mixing renames with behavior changes in the same edit.

## Completion check

`revive -config revive.toml ./...` reports no `var-naming` diagnostic and the
package compiles with tests green.
