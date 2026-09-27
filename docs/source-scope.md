# Repository source scope

`grepple sources explain` makes repository-owned source selection observable before an agent treats missing results as evidence.

```bash
grepple sources explain .
grepple sources explain --json --production-only src services
```

The `grepple-source-scope-v2` report includes the discovered `grepple.json` path and SHA-256 digest, whether configured ignores are active, selected and excluded file totals, exclusion counts by reason, metadata-backed source classifications, and deterministic per-path decisions. Infrastructure patterns such as `.git/**`, `.grepple/**`, and `.worktrees/**` are always listed as unconditional exclusions and are not traversed, so cache or artifact presence cannot change reported file totals. Unreadable or symlinked subtrees are reported separately as omissions rather than misrepresented as known file counts.

## Scope controls

- `--no-config-ignore` loads repository configuration, including server and spill settings, but disables `ignore.paths`.
- `--no-repo-config` bypasses repository-owned behavior entirely. User authentication in `~/.grepple/config.json` remains active.
- `--production-only` excludes non-production classifications during recursive discovery.
- Explicit files retain precedence over configured ignores and `--production-only`; the command emits a bypass notice.

These are global flags and may appear with search, graph, GritQL, ask, or architecture commands. Artifact reruns and generated continuation commands retain the active scope flags.

## Classifications

Classification comes from each file's current `grepple.yaml` entry rather than path or filename conventions. `grepple init` asks the configured model to classify every authoritative file from source evidence using exactly:

- `production`: application or library source used in normal operation;
- `test`: test implementation or test-only support;
- `fixture`: examples, samples, or fixture data/source;
- `generated`: generated or build-produced source;
- `vendor`: vendored third-party source;
- `unknown`: evidence is insufficient or no trustworthy classification is available.

A classification is trusted only when its path is a direct file entry, its kind is valid, and its recorded SHA-256 checksum matches current content. Missing metadata, omitted kinds, invalid kinds, and stale checksums classify the file as `unknown`; Grepple does not infer production status from a filename. Run `grepple init` after source changes to refresh stale or missing descriptions, checksums, and kinds (or `--only-directory PATH` to limit the refresh). Classification is source-scope metadata, not an ownership or deployment verdict. Use the complete universe unless the task explicitly asks about production code.
