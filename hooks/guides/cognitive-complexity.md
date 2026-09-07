# cognitive-complexity: keep functions easy to hold in your head (max 15)

## Goal

No function exceeds cognitive complexity 15 (nesting, branches, and recursion
each add cost). A function over the limit is doing too many decisions for a
reader — or an agent — to reason about safely.

## Preferred approach

1. Identify the hot region: deeply nested `if`/`for` chains and long branchy
   bodies. The diagnostic reports the measured score.
2. Extract the innermost coherent step into a well-named helper — prefer
   extracting *meaningful units* (a filter, a conversion, a validation) over
   arbitrary line ranges.
3. Prefer early returns to nested `if/else` pyramids; each guard clause drops
   a nesting level.
4. Replace branch cascades with tables or maps when the branches pick values.
5. Keep behavior identical: same errors, same ordering, same side effects.
   Run the package tests plus any test that names the function.

## Not acceptable

- Splitting into helpers named `foo1`/`foo2`/`runPart2` that hide the logic.
- Moving complexity sideways into a callback, closure, or goroutine that the
  rule cannot see but a reader still must.
- Raising the threshold in `revive.toml` or suppressing with `//nolint`.
- Changing behavior to reduce the score.

## Completion check

`revive -config revive.toml ./...` reports the function at ≤ 15, tests are
green, and the new helpers read as a table of contents for the original flow.
