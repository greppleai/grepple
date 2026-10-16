# Language-native scope selector contract

## Decision

Selectors are parser-owned stable public names, but their declaration names follow the source language rather than one cross-language abstraction. They are **not** raw tree-sitter node kinds. A token remains valid only when the current parser verifies the named construct and exact inclusive span in that language:

```text
TypeScript: formatUser @function:8-12
Go:         Run @func:2-5
Python:     run @def:1-3
Rust:       run @fn:1-3
```

Control-flow and content names already describe their source form (`if`, `for`, `section`, `object`, etc.) and remain stable where the language spelling is shared. The parser must determine the effective language from the syntax node (including Svelte injected trees), not file extension or outline display kind.

## Proposed declaration vocabulary

| Effective language | named function token | method/constructor token | type tokens |
| --- | --- | --- | --- |
| Go, Swift | `func` | `method` | `struct`, `interface` |
| JavaScript, TypeScript, TSX, PHP, Dart | `function` | `method` | `class`, `interface`, `enum` where supported |
| Python | `def` | `method` | `class` |
| Kotlin | `fun` | `method` when inside a class | `class` |
| Rust | `fn` | `method` in `impl` | `struct`, `trait`, `enum` |
| C, C++ | `function` | `method` where the adapter proves a member | `struct`, `class`, `enum` |
| Java, C# | `method` for callable declarations | `method` | `class`, `interface`, `enum`, `struct` where supported |
| Shell | `function` | n/a | n/a |
| CSS | n/a | n/a | `rule`, `property` |
| HCL | n/a | n/a | `block` |
| Svelte | injected language mapping | injected language mapping | `snippet`; template control flow remains `if`/`for` |
| Markdown/JSON/YAML | n/a | n/a | `section`, `object`, `array`, `mapping`, `sequence`, `document` |

A lambda/closure uses the effective language's function token only where the parser already recognizes it as a selectable scope. Unsupported declaration forms remain unlabelled. `trait` is intentionally added only when Rust adapter support and tests prove it; no token is emitted merely because outline text calls something a type.

## Compatibility and migration

The current normalized tokens (`func`, `class`, `method`, and peers) are a published local `--at` interface. New native output must not strand a user with an old copied token:

1. Parser output and presentation emit the new native kind after an explicit compatibility release.
2. The resolver accepts legacy normalized aliases **only for the effective language and construct they previously denoted**. An alias is never accepted when it could identify a different native construct on the same span.
3. Typed `ScopeLineRange.Kind` changes only in the designated breaking/additive version; JSON/wire capability/versioning and Go API behavior need an explicit compatibility decision before alteration.
4. `--at PATH:LINE[-END]`, remote rejection and malformed/stale/ambiguous selector failures are unchanged. Remote/artifact output never presents a local selector.

## Required implementation evidence

For every emitted token, prove exact and shorthand local `--at` round trips, wrong-native-kind rejection, legacy-alias policy, malformed/error rejection and same-line ambiguity. Test Go, JavaScript/TypeScript, Python, Rust, a JVM/C-family language, Svelte injection, Markdown, JSON and YAML; registry-wide tests must report unsupported adapters instead of fabricating a name. Preserve one parse per local presentation batch and never derive a selector from a renderer's `Symbol.Kind` or graph span.