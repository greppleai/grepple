# Resolve the reported revive rule at its cause

## Goal

Understand the named rule and make the smallest behavior-preserving change that
improves the code for the reason the rule exists.

## Preferred approach

1. Read the diagnostic, the reported `file:line:column`, and the rule name.
2. Inspect the surrounding implementation and relevant tests before editing.
3. Fix the underlying design or correctness issue rather than only changing
   syntax around the reported token.
4. Preserve public APIs, behavior, error handling, cancellation, cleanup, and
   ordering unless the task explicitly requires a change.
5. Run the narrowest relevant test (`go test ./internal/<pkg>/`) and rerun
   revive (`revive -config revive.toml ./...`).

## Not acceptable

- Adding `//nolint`, disabling the rule, or weakening `revive.toml` without
  explicit approval.
- Dead code, renamed-away identifiers, or `_ = x` assignments used only to
  silence analysis.
- Arbitrary helper extraction, renaming, file movement, or abstraction that
  does not clarify ownership.
- Removing intentional functionality or checks.
- Fixing unrelated diagnostics in the same pass.

## Completion check

The diagnostic is gone because its underlying issue was corrected, behavior
remains covered by tests, and the resulting code is simpler to explain.
