# unused-parameter: drop or underscore unused parameters

## Goal

A named parameter that is never read is noise and often signals a leftover from
refactoring. The signature should show which inputs actually matter.

## Preferred approach

1. If the parameter is genuinely unneeded and the function is not satisfying
   an interface/callback signature, remove it and update callers.
2. If the signature is fixed (interface implementation, handler shape, test
   helper), rename the parameter to `_`: `func (d *fake) Get(_ string)`.
3. For methods where the receiver is unused, the same applies to the receiver:
   prefer removing it or using `func (*fakeShard) handler()`.

## Not acceptable

- Reading the parameter into `_` inside the body to fake usage.
- Removing a parameter that a caller passes for a reason (check all callers).
- Widening the change into a signature redesign beyond the reported function.

## Completion check

`revive -config revive.toml ./...` reports no `unused-parameter` diagnostic
and the package still builds and tests green (`go test ./internal/<pkg>/`).
