# Grepple project hooks

This directory is a standalone Go module containing the project-local Pi hooks.
It intentionally does not import the main `grepple` module, so the hook runtime
can be replaced or removed without restructuring application packages.

## Layout

- `cmd/pi-hook` — command-hook protocol entry point.
- `cmd/mermaid-code` — standalone Mermaid structure and flow checker/generator.
- `internal/pihooks` — grep guard, lint orchestration, feedback, and analyzers.
- `internal/mermaidcode` — tree-sitter Go/TypeScript schema analysis.
- `guides` — remediation text returned by the Stop hook.

Pi invokes `make -C hooks run-…` from `.pi/settings.json`. In Go modules the
Stop lint hook runs formatting, revive, and custom Go analyzers before Mermaid
validation. Without `go.mod`, it skips Go tooling but still validates Mermaid
schemas; a schema-free project returns no output and does not require revive.
The hook cheaply scans for Mermaid schema files and only loads Go/TypeScript
sources when one of these suffixes exists:

- `*.class.mmd`
- `*.structure.mmd`
- `*.flow.mmd`

Class and structure files use the class schema checker; flow files use the call
flow checker. This repository keeps all generated artifacts under root `.grepple/`.
The hook discovers `*.package` and `*.workspace` bundle directories there, validates
each entire bundle with its canonical self-locating checker, and skips independent
validation of generated files inside those directories. Generated `.grepple/` content
is excluded from source analysis but not from schema discovery. Mismatches are returned
as bounded `mermaid-code` diagnostics at diagram lines or bundle manifests.

## Mermaid CLI

```bash
make -C hooks build

hooks/bin/mermaid-code check structure .grepple/service.structure.mmd .
hooks/bin/mermaid-code check flow .grepple/service.flow.mmd .
hooks/bin/mermaid-code generate package internal/service --output .grepple/service.package.mmd
hooks/bin/mermaid-code generate package internal/service --format bundle --output .grepple/service.package
hooks/bin/mermaid-code check package .grepple/service.package
hooks/bin/mermaid-code generate workspace . --output .grepple/project.workspace
hooks/bin/mermaid-code check workspace .grepple/project.workspace
hooks/bin/mermaid-code generate class internal/service/service.go Service --source . --output .grepple/service.class.mmd
hooks/bin/mermaid-code generate flow internal/service/service.go Service.Run --source . --output .grepple/service.flow.mmd
```

`generate package <source-directory> [--output file]` retains the legacy one-file output.
Adding `--format bundle --output <bundle-directory>` writes a canonical machine-generated
package bundle containing exactly `manifest.json`, `overview.mmd`, and `structure.mmd`.
The manifest is stable JSON for one syntax-derived semantic IR. Its package identity includes
a normalized `sourceDirectory`, mechanically relative to the selected package's `go.mod` root.
Its `syntactic-union-conflicts-rejected` build scope includes every non-test Go source file
without consulting runtime GOOS/GOARCH: valid leading Go build constraints and standard
filename-derived GOOS/GOARCH constraints are recorded per file and summarized as a sorted
union. Package-level type/function conflicts and duplicate methods anywhere in that union are
rejected with both source locations. The manifest also records files, types, exact members and tags, exported functions, routes, evidenced
local relations, summary counts, and its SHA-256 digest. `structure.mmd` is exhaustive; `overview.mmd` is
an LR projection grouped into syntax-derived namespaces. It lists every exported package
function, every interface method, and all named fields on tagged data-contract structs;
other declarations remain compact. Overview relations sharing from/to/kind/cardinality
are collapsed (up to three exact evidence labels, then deterministic per-category counts),
while the manifest and structure retain every exact relation. A mechanically extracted
first sentence from a conventional Go `Package name ...` comment is included when present.
`check package <bundle-directory> [source-directory]` regenerates and validates the bundle,
using the manifest's strict project-relative source metadata when source is omitted. An explicit
source remains supported and only succeeds when it produces the same canonical artifacts. It
rejects missing or extra files and byte-compares every artifact. Bundle generation requires
and rename/rollback. It refuses unexpected existing entries instead of deleting user content.
Package selection rejects empty, mixed-package, non-directory, module-less, and
TypeScript-only selectors.

`generate workspace <root> --output <bundle-directory>` writes exactly `manifest.json` and
`overview.mmd`. It recursively discovers root and nested `go.mod` files, but scans each module
without crossing into a nested module, and inventories directories containing direct non-test
Go files. The stable manifest records module/import paths and root-relative directories, exact
package-doc first sentences when present, package counts and Fiber route facts, local and
standard-library imports, and external imports mapped to the longest boundary-matching
`require` module path. Unmatched external imports are explicitly retained. Scope metadata
records the same syntactic build-tag union policy as package bundles and the excluded
directories/symlinks. Its digest covers the semantic model with only the digest field blanked.

The deterministic `flowchart LR` overview groups packages by module, marks `package main`
entrypoints mechanically, renders local edges, collapses each package-to-external-module edge,
and attaches at most five exact route facts plus a machine count for any remainder. It contains
no inferred descriptions. `check workspace <bundle-directory> [root]` strictly regenerates and
byte-compares both files, rejecting missing/extra entries. When root is omitted it uses strict
root metadata anchored to the containing Go module. Workspace writes use directory-level
temporary generation and rename/rollback and refuse symlink outputs or unknown existing files.

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
strictly reject malformed, duplicate, or conflicting metadata. Canonical bundles use these
defaults while retaining kind, file, tag, underlying-type, route, and file-local facts.
Alias and named-type nodes can opt into exact underlying Go type validation with
`%% grepple:underlying <Type> <normalized-Go-type-expression>`; generated `<<alias>>` and
`<<type>>` nodes emit this metadata. The legacy `pi` prefix is accepted.

Go structure diagrams can require exact struct tags with
`%% grepple:struct-tag <Type> <Field> <Go-quoted-string>` (including an explicit empty tag).
Fiber registrations can be checked with `%% grepple:route <METHOD> <PATH> <Type.method>`.
Route extraction recognizes HTTP registrations on `*fiber.App` parameters only when the
parameter qualifier resolves, including aliases, to `github.com/gofiber/fiber/v2` or `/v3`;
lookalike packages and unrelated `Get` methods are ignored. Underlying-type, route, and
struct-tag metadata use strict parsing and reject duplicate directives.

Relative TypeScript imports are followed across supported TS extensions and index files.
Generated diagrams emit applicable scope, file, export, tag, and file-local metadata and
validate themselves before being written. Namespaced metadata is strict while ordinary
Mermaid comments remain compatible. See `guides/mermaid-code.md` for full syntax, scope
resolution, completeness, underlying-type, route, and struct-tag details.

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
