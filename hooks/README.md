# Grepple project hooks

This directory is a small Go module containing project-local Pi hook orchestration.

Source coverage deduplication and its Pi SessionStart/PostCompact cache-invalidation commands have been removed. Grepple renders the complete selected source and edit anchors on each read, subject to explicit output, spill, and paging limits. The Pi configuration retains independent PreToolUse grep-guard and Stop lint/check hooks; these do not track source coverage or substitute for removed invalidation hooks.

## Layout

- `cmd/pi-hook` — command-hook protocol entry point.
- `internal/pihooks` — grep guard, lint orchestration, feedback, and adapters.
- `guides` — remediation text returned by the Stop hook.

Pi invokes `make -C hooks run-…` from `.pi/settings.json`. In Go modules the Stop lint hook checks formatting with read-only `gofmt -l`, runs Revive, and adapts the repository-owned GritQL `same-file-struct-methods` relation into diagnostic feedback. It never formats source or inserts annotations. Without `go.mod`, it returns no output.

Hexagonal policy belongs to `grepple-backend/.grepple/hooks/`, not the CLI repository.
The hook cheaply scans for Mermaid schema files and only loads Go/TypeScript
sources when one of these suffixes exists:

- `*.class.mmd`
- `*.structure.mmd`
- `*.flow.mmd`

Class and structure files use the class schema checker; flow files use the call
flow checker. Generated repository orientation is provided dynamically by `grepple architecture`; the Stop hook does not discover or validate generated package/workspace directories.

## Mermaid CLI

```bash
make build

bin/grepple architecture directory --depth 2 .
bin/grepple graph resolve --symbol Service .
bin/grepple extract structure internal/service --entry Service --source . --output .grepple/service.class.mmd
bin/grepple extract flow internal/service --entry Service.Run --source . --output .grepple/service.flow.mmd
bin/grepple extract check structure .grepple/service.class.mmd .
bin/grepple extract check flow .grepple/service.flow.mmd .
```

Focused structure and flow generation require `--entry` or `--at PATH:LINE`. `--source` defines the source universe; `--depth` and `--max-nodes` keep generated Mermaid bounded.

`structure` is an alias for `class`. Class diagrams may include one `direction LR|RL|TB|BT`
statement and non-nested `namespace <identifier> { ... }` grouping blocks; namespaces do not
qualify class identity. Conservative `class ID["display label"]` declarations retain `ID`
for metadata, relations, and source validation while storing the label for display. Generated
Go aliases and named types use this form to display `Name = Underlying`. The parser preserves
optional relation labels and accepts dependency arrows `..>` and `<..`. See the guide for the
exact grammar.

Go structs use `<<struct>>` (which implies
Go); interfaces use `<<interface>>`. Nodes allow additional code members by default;
`<<exact>>` opts a node into complete field-and-method matching. `<<go>>` and
`<<typescript>>` disambiguate mixed roots. Go package selection uses `%% grepple:package`, and flow symbol/language
selection uses `%% grepple:symbol` and `%% grepple:language` metadata. Generated TypeScript
nodes use `%% grepple:module`, and all generated class/structure nodes use
`%% grepple:file`, with stable project-relative paths. The opt-in
`%% grepple:complete-package <exact-go-import-path>` checks package completeness: every
package-level Go declaration and exported package function in that exact import path must have a package-scoped
node. Function-local types and unexported functions are excluded, and omissions are reported deterministically.
`%% grepple:filelocal` requires the exact contiguous declaration marker
`//grepple:filelocal`; validation also rejects structural references from other files in
the same exact package. Generated Go nodes include file-local metadata when marked and use
exact module import paths for package metadata. One-package schemas can replace repeated
scope and language metadata with `%% grepple:package-default <exact-go-import-path>` and
`%% grepple:language-default go|typescript`; `%% grepple:exact-default` makes only
non-function struct/interface nodes exact. Defaults are finalized independent of order and
strictly reject malformed, duplicate, or conflicting metadata.
Alias and named-type nodes can opt into exact underlying Go type validation with
`%% grepple:underlying <Type> <normalized-Go-type-expression>`; generated `<<alias>>` and
`<<type>>` nodes emit this metadata. The legacy `pi` prefix is accepted.

Go structure diagrams can require exact struct tags with
`%% grepple:struct-tag <Type> <Field> <Go-quoted-string>` (including an explicit empty tag).
Underlying-type and struct-tag metadata use strict parsing and reject duplicate directives.

Relative TypeScript imports are followed across supported TS extensions and index files.
Generated diagrams emit applicable scope, file, export, tag, and file-local metadata and
validate themselves before being written. Namespaced metadata is strict while ordinary
Mermaid comments remain compatible. See `guides/mermaid-code.md` for full syntax, scope
resolution, completeness, underlying-type, and struct-tag details.

## Development

```bash
make -C hooks build
make -C hooks test
make -C hooks lint
go -C hooks vet ./...
go -C hooks test -race ./...
```

The root `make build`, `make test`, and `make lint` targets include the normal
hook build, test, and lint steps.
