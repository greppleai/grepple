# errorf: use fmt.Errorf instead of errors.New(fmt.Sprintf(...))

## Goal

Error construction uses `fmt.Errorf` directly — one allocation, one call, and
`%w` support for wrapping.

## Preferred approach

- `errors.New(fmt.Sprintf("...%v", x))` → `fmt.Errorf("...%v", x)`
- When wrapping an error, use `%w`: `fmt.Errorf("index %s: %w", repo, err)`.
- If the message has no formatting verbs, drop to `errors.New("...")` (or
  `fmt.Errorf` stays fine if the codebase locally prefers it — but do not
  switch back and forth).

## Not acceptable

- Keeping `errors.New(fmt.Sprintf(...))` and suppressing the rule.
- Changing the error message text while converting (callers may match on it).
- Replacing `%v` with `%w` on a non-error argument.

## Completion check

`revive -config revive.toml ./...` reports no `errorf` diagnostic and error
messages are byte-identical to before.
