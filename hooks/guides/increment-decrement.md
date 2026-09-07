# increment-decrement: use i++ / i--

## Goal

Use Go's idiomatic increment/decrement statements instead of `+= 1` / `-= 1`.

## Preferred approach

- `i += 1` → `i++`
- `i -= 1` → `i--`

Only single-step statements are flagged; `i += 2` and compound expressions are
fine as-is.

## Not acceptable

- Rewriting surrounding loop logic to make the lint pass.
- Changing `+= 1` on a non-integer type — check the type first (the rule
  only fires on ints, but confirm before editing shared arithmetic).

## Completion check

`revive -config revive.toml ./...` reports no `increment-decrement` diagnostic
and the loop/step behavior is unchanged.
