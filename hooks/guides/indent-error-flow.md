# indent-error-flow: keep the happy path unindented

## Goal

Error handling takes the indented branch; the success path stays at the left
margin. `if err == nil { ... } else { ... }` inverts that and buries the happy
path.

## Preferred approach

```go
// before
if err == nil {
    use(v)
} else {
    return err
}

// after
if err != nil {
    return err
}
use(v)
```

1. Invert the condition, put the error/early-return branch first.
2. Keep the else-less form: after a returning `if`, drop the `else` block and
   outdent its body.

## Not acceptable

- Reordering side effects or changing which branch returns.
- Deep restructuring beyond the reported if/else.

## Completion check

`revive -config revive.toml ./...` reports no `indent-error-flow` diagnostic
and the function's behavior is unchanged (tests green).
