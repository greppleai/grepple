# error-strings: error messages are lowercase, no punctuation

## Goal

Error strings are lowercase and end without punctuation, because they are
composed: `fmt.Errorf("refresh login: %w", err)` reads as one sentence chain.

## Preferred approach

- `errors.New("Invalid token.")` → `errors.New("invalid token")`
- Lowercase the first word unless it is a proper noun or initialism
  (`GitHub`, `URL`).
- Strip trailing `.`, `:`, `!`, `\n`.

## Not acceptable

- Changing message content beyond casing/punctuation — tests and operators
  may match on these strings (`grepple -F "invalid token" --remote` first).
- Capitalizing for style consistency with nearby non-error strings.

## Completion check

`revive -config revive.toml ./...` reports no `error-strings` diagnostic and
`go test ./...` passes (message-matching tests included).
