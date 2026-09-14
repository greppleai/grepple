# Repository source scope

`grepple sources explain` makes repository-owned source selection observable before an agent treats missing results as evidence.

```bash
grepple sources explain --compact .
grepple sources explain --json --production-only src services
```

The `grepple-source-scope-v1` report includes the discovered `grepple.json` path and SHA-256 digest, whether configured ignores are active, selected and excluded file totals, exclusion counts by reason, source classifications, and deterministic per-path decisions. Infrastructure patterns such as `.git/**`, `.grepple/**`, and `.worktrees/**` are always listed as unconditional exclusions and are not traversed, so cache or artifact presence cannot change reported file totals. Unreadable or symlinked subtrees are reported separately as omissions rather than misrepresented as known file counts.

## Scope controls

- `--no-config-ignore` loads repository configuration, including server and spill settings, but disables `ignore.paths`.
- `--no-repo-config` bypasses repository-owned behavior entirely. User authentication in `~/.grepple/config.json` remains active.
- `--production-only` excludes non-production classifications during recursive discovery.
- Explicit files retain precedence over configured ignores and `--production-only`; the command emits a bypass notice.

These are global flags and may appear with search, graph, boundaries, GritQL, focused extraction, or architecture commands. Artifact reruns and generated continuation commands retain the active scope flags.

## Classifications

Classification is conservative and language-neutral:

- `test`: conventional test directories and filenames such as `tests/`, `_test.go`, `.test.*`, and `.spec.*`;
- `fixture`: `testdata/`, fixture, example, and sample trees;
- `generated`: conventional generated/build directory and filename forms;
- `vendor`: vendored and third-party dependency trees;
- `production`: everything not confidently classified above.

Classification is source-scope metadata, not an ownership or deployment verdict. Use the complete universe unless the task explicitly asks about production code. Repository-specific exceptions belong in `grepple.json` ignore policy rather than in inferred package semantics.
