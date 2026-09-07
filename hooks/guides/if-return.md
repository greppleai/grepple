# if-return: collapse redundant if/else return

## Goal

A boolean computed by an `if cond { return true }; return false` (or an
if/else returning values both ways) collapses into a direct `return`.

## Preferred approach

```go
// before
if x > 0 {
    return true
}
return false

// after
return x > 0
```

For value-returning if/else where revive suggests it, move the final `return`
out of the else and drop the else block.

## Not acceptable

- Collapsing when the branches carry comments or side effects that would be
  lost — keep the explicit form instead (the rule yields to clarity).
- Changing the condition while collapsing.

## Completion check

`revive -config revive.toml ./...` reports no `if-return` diagnostic and the
function is behavior-identical.
