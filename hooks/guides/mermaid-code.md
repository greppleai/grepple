# Mermaid code schemas

The main Grepple `extract` package checks and generates class/structure diagrams and call flowcharts
from source parsed with Tree-sitter. Focused extraction supports Go and TypeScript-family sources; canonical package and workspace bundles remain Go-specific. Generic callable declarations and calls use the normalized `parser.NavigationGraph` shared with local code navigation. Malformed source reports its path and first syntax error location.

## Automatic Stop validation

The Stop hook automatically checks files ending in `.class.mmd`,
`.structure.mmd`, or `.flow.mmd`, and discovers canonical bundles at
`*.package` and `*.workspace` directories. This repository exports all generated
artifacts beneath root `.grepple/`; generated content there is excluded from source
analysis but remains visible to schema discovery. Bundle directories are checked as
a unit with the matching canonical, self-locating checker; their generated children are not
independently treated as legacy diagrams. Bundle failures become bounded `mermaid-code`
diagnostics anchored to the manifest, without preventing checks of unrelated bundle or legacy
schemas. It first performs a cheap schema/bundle scan; when none exist, no Go/TypeScript
source discovery or parsing occurs. This also runs in TypeScript-only projects without
`go.mod`, where revive and other Go tooling are not required. Class and structure suffixes
select the class schema, while the flow suffix selects the call-flow schema. Mismatches appear
as `mermaid-code` lint diagnostics at the corresponding diagram line. Syntax and
source-analysis failures are returned as bounded Stop feedback.

## CLI

```bash
bin/grepple extract check structure .grepple/model.class.mmd .
bin/grepple extract check structure .grepple/model.structure.mmd .
bin/grepple extract check flow .grepple/calls.flow.mmd .

bin/grepple extract structure internal/service --bundle --output .grepple/service.package
bin/grepple extract check package .grepple/service.package
bin/grepple extract structure service.go --entry Service --source . --output .grepple/service.structure.mmd
bin/grepple extract flow service.go --entry Service.Run --source . --output .grepple/service.flow.mmd
```

Focused flow generation is deterministically bounded. When resolvable calls exceed `--max-nodes`, generation keeps the selected prefix and emits `%% grepple:truncated max-nodes N`; the checker accepts this explicit warning while continuing to validate every emitted node and edge.

`extract structure <source-directory>` generates the complete package diagram. Add `--entry`
for a focused type diagram. The `--bundle` form
always contains exactly `manifest.json`, `overview.mmd`, and `structure.mmd`. The manifest
records the normalized package source directory relative to its `go.mod` root. Thus
`extract check package <bundle-directory> [source-directory]` can safely self-locate its source when the
second argument is omitted; an explicit source must reproduce the canonical manifest. Checking
regenerates and byte-compares all three canonical artifacts. Bundle writes stage all
artifacts in a sibling temporary directory, then rename the complete directory into place; an
existing generated bundle is renamed aside and restored if commit fails. Unexpected entries
are rejected before staging and are never deleted.

The overview summary note is emitted immediately after `direction LR`. It begins with the
exact mechanically extracted first sentence of a conventional `Package name ...` Go package
comment when one exists; no replacement prose is synthesized. The overview includes every
exported package function, all interface methods, and every named field of a tagged
data-contract struct. Other declarations stay compact. Relations are grouped by
from/to/kind/cardinality: groups of up to three retain their sorted exact `via` evidence;
larger groups use deterministic evidence-category counts. `manifest.json` and
`structure.mmd` retain every exact relation.

Manifest JSON uses these lossless defaults to reduce repetition: a member without `file`
inherits its declaration's file; absent `parameters` means no parameters; absent `result`
means no result; absent `structTag` means the source field had no tag (an object with an empty
`value` represents an explicitly empty tag). Declaration/member visibility and exportedness
are derived from Go identifier capitalization and are not duplicated. Package-level exported
entries in `exportedFunctions` use kind `function`, never `method`. Empty build-constraint arrays
are omitted per source file; top-level semantic arrays remain present for a stable shape. The
`scope.buildTags` value `syntactic-union-conflicts-rejected` means generation considers every
non-test Go file without evaluating the host or environment GOOS/GOARCH. Only valid constraints
in Go's leading build header are recorded; standard filename GOOS/GOARCH suffix constraints are
added mechanically. `syntacticBuildTagUnion` is their sorted union. Because mutually exclusive
files are represented together, duplicate package-level types/functions and duplicate methods
are rejected rather than overwritten or merged, and diagnostics report both source locations.

## Class and structure conventions

The checker accepts this conservative class-diagram projection grammar:

```text
classDiagram
[direction LR|RL|TB|BT]
{ class-declaration | stereotype | metadata | relation | namespace-block }*

class-declaration = class identifier [ ["display label"] ] [ { members } ]

namespace-block = namespace identifier {
  { class-declaration | stereotype | metadata }*
}
relation = identifier ["multiplicity"] operator ["multiplicity"] identifier
           [ : label ]
operator = <|.. | ..|> | <|-- | --|> | ..> | <.. | --> | <--
         | *-- | o-- | --* | --o | --
```

At most one direction statement is allowed. Namespaces cannot nest and are readable
grouping only: class names keep their global identity, so duplicate names across blocks
are rejected and relations continue to use the unqualified class name. A conservative
`class ID["display label"]` stores the display label but keeps `ID` as the validation,
metadata, note, and relation identity. Generated labels entity-escape Mermaid-significant
characters. Class bodies may appear inside a namespace; external stereotypes and
`grepple`/legacy `pi` metadata work there as at top level. Relations must remain outside
namespace and class bodies. Dependency arrows (`..>` and `<..`) are checked like directed
associations. An optional relation label after `:` is preserved by the parser but does not
alter source validation.

Class nodes default to TypeScript classes. `<<struct>>`, `<<alias>>`, and `<<type>>`
intrinsically mean a Go struct, type alias, and other named type respectively, while
`<<interface>>` identifies interfaces in either language. `<<go>>` and `<<typescript>>`
select the source language when a node name occurs in mixed-language roots; an
unqualified cross-language collision is reported as ambiguous. Contradictory kind or
language stereotypes (for example, `<<alias>>` plus `<<type>>`, or `<<struct>>` plus
`<<typescript>>`) are rejected regardless of their order. Generated class diagrams
language stereotypes (for example, `<<alias>>` plus `<<type>>`, or `<<struct>>` plus
`<<typescript>>`) are rejected regardless of their order. Generated class diagrams
emit declaration-kind stereotypes and a language stereotype so aliases and named
types round-trip. Function nodes retain `<<function>>` and can also carry a language
stereotype.

Generated aliases and named types display their identity and exact underlying expression
directly as `class Name["Name = Underlying"]`; exhaustive diagrams still carry the
underlying metadata below for validation:
```mermaid
%% grepple:underlying FileResult grepple.FileResult
%% grepple:underlying IDs []ID
%% grepple:underlying Handler func(string)error
```

The expression is parsed as a Go type expression and compared after Go whitespace
normalization with the declaration's exact alias target or named underlying type. The
directive is only valid for Go `<<alias>>` and `<<type>>` nodes. Duplicate and malformed
directives are rejected. The legacy `%% pi:underlying` spelling is accepted; generation
always emits `grepple:underlying` for alias and named-type nodes.

By default, listed members are a required subset and additional code members are
allowed. Add `<<exact>> NodeName` when the node is an architectural boundary whose
complete field and method surface must match the diagram. Exact nodes report every
additional member as a `mermaid-code` mismatch.

Go declarations and calls are isolated by source directory plus parsed package
clause. Broad roots containing the same symbol in multiple packages are
ambiguous rather than merged. Narrow the source roots, or use package metadata:

```mermaid
%% grepple:package Worker service
```

Class generation emits package/module metadata plus `%% grepple:file Node path` for
each declaration. It fails if a selected member cannot be represented safely in
the supported Mermaid member syntax.

Go member visibility follows identifier capitalization (`+` exported, `-`
unexported). Receiver methods are merged into their package-local named type.
Methods from embedded local structs and interfaces are expanded transitively
for structural interface checks. Pointer and value receivers are intentionally
treated as one type family. Qualified embedded types retain their qualification
and are not linked to unrelated local types. Go imports use the local package
binding; `<<export>>` means a package-level exported identifier. Go has no
default export.

Go types are compared syntactically after whitespace normalization; Go
primitives remain exact and do not receive TypeScript aliases. Variadic
parameters use `...T`. Multiple returns use `tuple~T,U~` in Mermaid. Slices,
arrays, maps, pointers, and generic type applications participate in association
checks.

Association cardinality is syntax-based. On the target end, an explicit `"*"`
requires a collection reference and an explicit `"1"` requires a non-collection
reference; omitting target multiplicity preserves the legacy behavior and accepts
either. Only `"1"` and `"*"` are supported multiplicities. TypeScript arrays,
`ReadonlyArray`, and `Set`, plus Go slices, arrays, and map values, are collection
syntax. For `-->`, `*--`, and `o--`, the owner is on the left and the target is on
the right; `<--`, `--*`, and `--o` reverse those roles. The same rules apply to
references in type members and function signatures.

Go struct field tags can be checked exactly with opt-in metadata:

```mermaid
%% grepple:struct-tag User Email "json:\"email,omitempty\" validate:\"required\""
```

The target must resolve to a Go struct and the field must be a named field on that
struct. The final argument is one Go-quoted string (an interpreted or raw string
literal); its unquoted value must exactly equal the source field tag. An explicit
empty source tag (`""`) is distinct from an absent tag and can be required with a
final `""`. Untagged fields are not checked unless a directive names them. Generated
Go structure diagrams emit one `grepple:struct-tag` directive for every present named
field tag, including explicit empty tags. The legacy `%% pi:struct-tag` spelling is
accepted, but generation always emits `grepple`.

Go Fiber routes can be checked with exact, opt-in metadata:

```mermaid
%% grepple:route GET /health Handler.health
%% grepple:route ALL /index Handler.handleIndex
```

The method must be an uppercase token and the path must contain no whitespace. The
checker recognizes HTTP-method calls made through a `*fiber.App` parameter inside a
Go method and resolves handler selectors on that method's receiver. Method, literal
path, and `Type.method` handler must all match exactly, in the same Go package as the
declared type node. Calls to similarly named methods on unrelated values are ignored.
Duplicate directives are rejected. The legacy `%% pi:route` prefix is accepted.

## Flow conventions

Flowcharts map readable node IDs with metadata:

```mermaid
%% grepple:symbol run Service.Run
%% grepple:language run go
%% grepple:package run service
```

Go flow generation traverses every resolvable call across supplied packages and
emits language and package metadata for every node. Bare calls resolve only
inside the caller's package. For sources beneath `go.mod`, imports resolve by
exact module path plus relative package directory. Explicit aliases are honored.
Import-path-basename matching is only a fallback for synthetic or no-module
fixtures.

TypeScript declarations and callable symbols are isolated by source module. An
overload set is merged only within one source module. Same-name declarations or
symbols in separate files are ambiguous; generation always starts from the
exact entry file. Bare TypeScript calls resolve only within their source module.
Qualified type references such as `Namespace.User` do not imply a relationship
to an unqualified local `User`.

## Discovery policy

Source discovery excludes symlinks, declaration files, generated Go files,
`_test.go`, and `.git`, `.worktrees`, `dist`, `generated`, `node_modules`, and
`vendor` directories. Go build tags are not evaluated: all non-test,
non-generated `.go` files in the selected package directory are treated as one
syntactic variant.

## Portable scope metadata

Generated TypeScript class nodes carry an exact source-module scope:

```mermaid
%% grepple:module User src/model/user.ts
```

Flow nodes use the node identifier as the metadata target:

```mermaid
%% grepple:module loadUser src/service/user.ts
```

Paths use `/` separators and are relative to the nearest language project root:
`go.mod` for Go, or `package.json`/`tsconfig.json` for TypeScript. When supplied
sources span multiple roots, paths are relative to their common source root. A
manually supplied normalized module suffix is accepted only when it identifies
exactly one analyzed TypeScript module; ambiguous suffixes produce a diagnostic.

For Go, `%% grepple:package` contains the exact module import path derived from the
nearest `go.mod`, such as `example.com/app/helper`. Existing package-name scopes
remain accepted when that package name is unambiguous. Synthetic source sets
without a module use an unambiguous source-root-relative package scope when
package names collide.

A one-package class schema may declare strict diagram-wide defaults:

```mermaid
%% grepple:package-default example.com/app/internal/service
%% grepple:language-default go
%% grepple:exact-default
```

`package-default` supplies the exact Go import path to every node, and
`language-default` must be exactly `go` or `typescript`. `exact-default` applies only
to non-function `<<struct>>` and `<<interface>>` declarations. Defaults are applied
after parsing, so their position relative to nodes and stereotypes does not change
the result. Each default may occur only once. Malformed defaults, duplicate defaults,
a Go package default combined with a TypeScript language default, and conflicting
per-node package/module/language metadata are rejected. Matching explicit metadata
is accepted but redundant. The legacy `pi` namespace is accepted for these defaults.
Canonical Go package bundles emit package and language defaults once in both Mermaid
artifacts and emit exact-default only in `structure.mmd`; node kind, file, struct-tag,
underlying-type, route, and file-local metadata remain explicit.

A class or structure schema can require package-level declaration completeness for one
exact Go import path:

```mermaid
%% grepple:complete-package example.com/app/internal/service
```

Every package-level struct, interface, alias, and other named type in that exact package
must have a diagram node scoped to the same package. Every exported package-level
function must also have a same-package `<<function>>` node; methods, unexported
functions, and `init` are excluded. Function-local type declarations are excluded.
Each uncovered declaration produces one diagnostic on the directive line, ordered by
normalized source path, declaration line, then symbol name. Nodes scoped to another
package do not satisfy completeness, even when they have the same name. The legacy
`%% pi:complete-package` prefix is accepted. Generation does not add this opt-in
directive automatically.

Class and structure nodes can additionally require their declaration's exact file:
```mermaid
%% grepple:file Worker internal/service/worker.go
```

The path must exactly equal the normalized project-relative source path. On an exact
Go node, listed members must be implemented in that file. Exactness still checks the
package-wide member set, so an additional method in another file is rejected.

Go structure nodes may additionally declare that a type is local to its declaration
file:

```mermaid
%% grepple:filelocal Worker
```

The declaration must have the exact marker comment `//grepple:filelocal` in its
contiguous leading comment block. Validation reports the first deterministic use of
the named type from another source file in the same exact Go package. References are
found structurally with tree-sitter, including receivers, fields, parameters, return
types, composite literals, and conversions; uses in the declaration file are allowed.
Package identity includes the source directory and package name, so an identically
named package in another directory is isolated. Generated Go diagrams emit file-local
metadata when the marker is present. The legacy `%% pi:filelocal` spelling remains
accepted, but generation always emits `grepple`.

Relative TypeScript imports are resolved syntactically for explicit `.ts`,
`.tsx`, `.mts`, and `.cts` paths, extensionless files, and matching `index`
files. Named aliases and default bindings are followed for generated class
dependencies and call flows. Bare symbols remain module-local and are never
matched to an unrelated module merely because the name is the same.

Repeated interfaces with the same name in one TypeScript module are merged.
Repeated declarations involving a class, or another incompatible kind, are
reported as incompatible declarations.

This remains syntax-based analysis, not a TypeScript semantic typechecker. It
does not implement `baseUrl`, `paths`, package export maps, JavaScript output
extension remapping, declaration-file resolution, namespace semantics, or
runtime/dynamic import resolution. Bare package specifiers are recorded but are
not traversed as local modules.

Go function and import nodes use the same exact `%% grepple:package` matching as type
nodes. An unexported Go interface method can be satisfied only by a method from
the same exact package identity; exported interface methods may participate in
cross-package structural matching.

TypeScript import and export aliases retain declaration identity. This includes
local export lists such as `export { finish as done }` followed by
`import { done as complete }`. Generated associations and heritage relations use
the declared target name while validation resolves the source-local alias.
Import-node `%% grepple:module` metadata identifies the importing module, so an
identically named import in another module cannot satisfy it.
