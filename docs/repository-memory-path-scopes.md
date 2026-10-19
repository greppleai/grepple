# Repository memory path scopes (proposed v1 extension)

This document freezes the contract for adding bounded path relevance to
repository-local memories. It is deliberately separate from
[repository memories](repository-memories.md) until the implementation and
examples land. A scope is **not** a checksum attachment and does not prove
that any current source bytes were reviewed.

## Header and CLI

The existing `grepple-memory-v1` header gains an optional `scopes` array of
canonical strings. It is distinct from `files`: every `files` attachment keeps
its mandatory SHA-256 checksum and optional line/symbol verification. Old
headers without `scopes` remain valid and must not be rewritten when read.

```yaml
schema: grepple-memory-v1
name: cli-skill-sync
description: Review affected skills after CLI changes.
scopes:
  - internal/cli/
  - internal/cli/*.go
  - internal/cli/**/*.go
```

`grepple memory store` and `grepple memory update` gain repeatable
`--scope PATH`. `store` accepts a bare repository-relative directory only as
an input convenience: after confirming it is an existing, non-symlinked
directory beneath the repository root, it serializes `internal/cli` as
`internal/cli/`. A bare name is never guessed to be a directory, and a bare
file remains an error; use `--file` for a checksum-current exact file
attachment.

`update` remains full replacement. A user who wants to retain scopes, labels,
or attachments supplies all of them again; unspecified old `scopes` are not
merged or retained. `recall` prints the stored frontmatter and body as today;
it may report declared scopes, but must not call a scope `current`, `stale`, or
checksum-verified.

A memory is discoverable when it has at least one `files` attachment, label,
or scope. A scope has no captured checksum and does not expand into one during
store, update, recall, or search.

## Canonical grammar

Scopes use case-sensitive, repository-root-relative `/` paths regardless of
host OS. They consist of one or more literal safe directory components followed
by exactly one of these suffixes:

| Stored scope | Meaning for an already selected result path |
| --- | --- |
| `path/` | any descendant file under `path/` |
| `path/*.go` | a direct child of `path/` whose basename matches `*.go` |
| `path/**/*.go` | a direct or nested child of `path/` whose basename matches `*.go` |

`path` is a literal prefix such as `internal/cli`; it cannot be empty. The
filename pattern in the second and third forms is a nonempty basename with one
or more `*` wildcards and literal UTF-8 text. `*` matches zero or more
non-`/` bytes. It cannot contain `/`, `**`, `?`, `[`, `]`, `{`, `}`, a backslash,
a colon, NUL, or control characters. Thus `*.go`, `*_test.go`, and `*` are
valid basename patterns; arbitrary shell globs and character classes are not.
The recursive token is only the exact middle component `**`; it is not accepted
at the end (`path/**`), in a literal prefix, or more than once. The direct form
does not match nested children; the recursive form does.

Literal prefix components use the existing repository-path safety rules and
also reject `.`, `..`, empty components, wildcard characters, absolute paths,
drive/colon forms, `.git`, `.grepple`, and descendants of either internal
directory. A scope may not target the repository root (`/`, `*`, or `**/*`).
At write time its literal prefix must exist as a non-symlinked directory reached
without following a symlink; it need not currently contain a matching file.
This validation does not enumerate matching files. On later recall/search a
removed prefix merely has no qualifying selected paths; stored scopes remain
readable.

Canonicalization preserves literal component spelling and the basename pattern,
adds the trailing slash for a directory scope, and rejects duplicate canonical
scopes. Limit one memory to 32 scopes, each at most 256 UTF-8 bytes. The fixed
literal prefix has at most 32 components; there is at most one recursive token
and the permitted terminal wildcard grammar bounds matching work. These limits
are in addition to the existing 8 KiB header and 64 KiB entry limits.

## Discovery, evidence and ranking

Scope matching is a pure comparison of a validated canonical scope and the
normalized root-relative `MatchedFile.Path` values already in the final local
content-result page. It does not enumerate a directory, expand a glob, read a
new candidate source, alter source policy, inspect query text/body text/context,
or consult local metadata. The selected path must already have an actual
matched `Matches[].Line`; structural `StartLine`/`EndLine` and surrounding
context are never scope evidence.

All relevance mechanisms are alternatives: a memory qualifies if any
checksum-current attachment, same-file current metadata-label predicate, or
path scope qualifies. A stale attachment does not qualify by itself, but does
not suppress independent scope or label evidence. Scope relevance means only
that the selected result path matches the stored pattern; it makes no statement
about source checksum freshness, line/symbol identity, metadata freshness, or
whether every file in the scope was reviewed.

Use the strongest evidence once per memory, in this deterministic order:

1. parser-reverified symbol attachment;
2. checksum-current line-range attachment intersecting an actual matched line;
3. checksum-current whole-file attachment;
4. checksum-current, file-owned metadata labels satisfying every populated
   label category on one selected file;
5. path-scope relevance.

Sort equal-strength memories by normalized name. Continue to show at most
three references, count only inspected qualified entries in `omitted`, and
retain incomplete-scan behavior. Scope-only matches use the same safe
name/description references already rendered today; implicit output must not
expose scope strings, fixed prefixes, stored labels, checksums, bodies, or a
new provenance field.

The existing local-only gates remain authoritative. Do not inspect scopes for
remote-only or mixed/explicit-server/repository/cursor searches, stdin,
files/files-with-matches, counts, outlines, `--at`, or explicit related modes.
No qualifying memory in a complete scan preserves existing text and JSON bytes.

## Test matrix

Implementation must add focused tests for:

- header round trips, legacy headers, duplicate/over-limit scopes, full-update
  replacement, help parsing, and recall without checksum-status claims;
- canonical directory input, direct and recursive basename patterns, nested
  paths, direct-vs-recursive behavior, case-sensitive comparison, empty/currently
  unmatched prefixes, unsafe roots/internal paths, malformed glob syntax,
  `..`, drives, separators, symlink prefixes, and bounded pattern length/depth;
- only-selected-page semantics under `--skip`/`--limit`, actual-match-line
  evidence, no structural/context widening, stale attachment plus independent
  scope, label ordering, deterministic ties, and top-three omission behavior;
- no source enumeration or wildcard expansion, remote/mixed/stdin/mode
  suppression, root-relative normalization from nested working directories,
  output/spill budgets, JSON parity, privacy, and telemetry isolation; and
- an end-to-end store/search/recall/update/delete flow, including a future file
  under a scope and the repository's `cli-skill-sync` migration after the
  behavior is implemented.
