# revive remediation guides

The prebuilt Go Stop hook maps a revive diagnostic's `RuleName` to a guide file:
rule `exported` → `exported.md`, `unused-parameter` → `unused-parameter.md`.
gofmt differences are auto-fixed by the hook itself and only reported; `format.md`
documents that policy for humans. When no matching file exists, the hook falls back
to `general.md`.

Comment-style rules (`exported`, `package-comments`) are merged per file into one
prompt mapped to `comments.md` — writing a file's documentation is one job, so the
agent gets every comment issue in that file at once instead of one rule per Stop.

Project analyzers in `internal/pihooks/` (for example, the tree-sitter-based
same-file struct-method analyzer) emit revive-shaped diagnostics with their own rule names and follow the same mapping:
`same-file-struct-methods` → `same-file-struct-methods.md`. They run after revive
and their diagnostics join the same report, grouping, and guide pipeline.

Keep each guide prescriptive and short: the design goal, the preferred fix shape,
unacceptable lint-silencing patterns, and the completion criteria.

The hook prioritizes the file with the most remaining diagnostics (ties broken by
path), then emits its first sorted group and one guide per Stop continuation. All
diagnostics of the group in the selected file are listed together. After the group
is fixed, the next Stop reruns revive and emits the next remaining group.
