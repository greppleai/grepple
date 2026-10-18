# Follow a verified scope from a line-only match

These commands use the current local file, not the displayed annotation as authority. Run them from **this `examples/` directory** after [building the binary](./README.md#setup). The plain TypeScript (`.ts`) example below runs from the repository root (`cd ..`) so its fixture path and displayed path are portable. Copy fresh `@KIND:START-END` tokens after changing a fixture; the line numbers shown here match these committed fixtures.

## TypeScript (`.ts`) and TSX (`.tsx`)

From the repository root:

```bash
cd ..
grepple --line-only -F 'logger.info' testdata/typescript/sample.ts
# The hit on line 10 currently ends in @func:8-12.
grepple --line-only --at testdata/typescript/sample.ts @func:8-12
# Equivalent shorthand when there is only one containing func at this line:
grepple --line-only --at testdata/typescript/sample.ts @func:10
cd examples
```

The first command prints the matching line, **not** the function's declaration; the verified reads return clean, anchored lines 8–12 (`formatUser`). The parser checks the kind and both inclusive bounds against the file again; stale bounds fail instead of silently shifting. One-line constructs such as an arrow function may produce `@func:18-18`; verified one-line reads do **not** expand to the enclosing method. For a method or class, use a current `@method:START-END` or `@class:START-END` token rather than guessing one from a function hit.

From `examples/`, TSX follows the same workflow:

```bash
grepple --line-only -F 'setPressed(true)' sample-files/Button.tsx
# Current matching line 12 ends in @func:11-14.
grepple --line-only --at sample-files/Button.tsx @func:11-14
```

The selector returns exactly the `handleClick` function, not the entire component. These examples work because TypeScript and TSX have parser adapters; they are not TypeScript type-checking or runtime call tracing. For caller questions, use `grepple graph resolve` followed by `graph callers` with the **graph's** `PATH:LINE` reference, not a scope token.

To constrain that graph to local, currently annotated files (run these from the repository root, substituting labels reported by `grepple overview` for your checkout):

```bash
grepple graph resolve --symbol ParseDocumentV1 --purpose implementation --domain-area directory-metadata internal/directorymeta
grepple graph callers --at internal/directorymeta/schema_v1.go:41 --purpose implementation --domain-area directory-metadata internal/directorymeta
```

A real `--at` location must be in a matching file. Repeat a flag to OR labels in that category; combining categories requires all labels on the same checksum-current file. Missing/stale entries are excluded (inspect `grepple verify`); a small `--max-files` cap is applied **after** filtering. Generated resolve follow-ups and truncation continuations retain these labels and the paths. This does not select remote `--repo` sources or make edges to excluded files complete.

## JSON, YAML and Markdown

The same two-token local selector works with verified content scopes:

```bash
grepple --line-only -F 'timeoutSeconds' sample-files/config.json
# @object:13-16 @object:1-17 (nearest limits object, then outer object)
grepple --line-only --at sample-files/config.json @object:13-16

grepple --line-only -F 'extraFlags' sample-files/values.yaml
# @mapping:34-37 @mapping:3-40
grepple --line-only --at sample-files/values.yaml @mapping:34-37

grepple --line-only -F 'minVersion' sample-files/guide.md
# @section:9-12
grepple --line-only --at sample-files/guide.md @section:9-12
```

For a machine-readable match, use `grepple --line-only --json-matches -F 'timeoutSeconds' sample-files/config.json`. Each match has literal `text` and separate typed `nearest` and `scope` (`kind`, `startLine`, `endLine`); do not parse an `@...` substring from source text. A nested same-kind shorthand like `@object:15` may be ambiguous: copy the explicit bounds of the desired object. Invalid syntax, unprovable bounds, unsupported/plain-text files and binary files cannot supply a verified selector. For an ordinary literal line range, use `grepple --line-only --at PATH:START-END` instead; legacy remote `PATH:LINE[-END]` also remains available, but **verified selectors are local-only**.

The live registry has code adapters for Go, Java, Kotlin, Dart, Swift, HCL, Svelte, CSS, JavaScript/JSX, TypeScript/TSX, Python, C#, C/C++, Rust, PHP and Shell, plus these Markdown/JSON/YAML scope classifiers. A language's available `@KIND` roles depend on confidently parsed syntax; not every grammar has every kind. `grepple languages` reports general feature availability, while [`docs/at-verified-scope-selectors.md`](../docs/at-verified-scope-selectors.md) records the selector contract. Reread clean source before `grepple write`: decorated search hashlines are display hints, not edit anchors.
