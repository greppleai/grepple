# receiver-naming: one receiver name per type

## Goal

All methods on a type use the same short receiver name (`s *shard`
everywhere, not `s` in one method and `state` in another), so readers can
pattern-match `s.` as "the receiver".

## Preferred approach

1. Look at the type's other methods and pick the majority receiver name:
   `grepple "func (.*shard)" internal/shard --line-only`.
2. Rename the outlier method's receiver to match, updating its body
   references only.
3. Keep it short (1–2 letters derived from the type), never `this`/`self`.

## Not acceptable

- Renaming the majority of methods to match one outlier.
- Changing receiver type (value vs pointer) — that alters semantics.

## Completion check

`revive -config revive.toml ./...` reports no `receiver-naming` diagnostic and
the package builds and tests green.
