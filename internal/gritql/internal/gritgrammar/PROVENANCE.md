# Vendored tree-sitter GritQL grammar provenance

- Upstream URL: <https://github.com/getgrit/tree-sitter-gritql.git>
- Commit: `f9d98660bd7ae78c9211cb52e295bcd6531a8121`
- Upstream relationship: derived from the exact gitlink used by `getgrit/gritql` commit `4ca283484ab9bd11ca3cbc9e14b5ad2db5d61a37`.
- Grepple patch: `languageName` additionally accepts `javascript`, `typescript`, `tsx`, `shell`, `dart`, `swift`, `svelte`, and `lua`; `predicateMatch` additionally accepts `emptyPredicate` as a direct right-hand side; `_pattern` additionally accepts `patternParent` for the direct `parent kind("function")` test. The evaluator implements both predicates without an external runtime.

## Files and SHA-256

All copied upstream files are covered by the exact vendored `LICENSE` (MIT, copyright 2024 Iuvo AI, Inc.). `binding.go` is the Grepple Go binding and is covered by the Grepple project license.

| File | Origin | SHA-256 |
| --- | --- | --- |
| `LICENSE` | `LICENSE` | `efa8bfef33f67df331a760fbadae76841d5ccab4135fa5c53e44b5b3253d55cb` |
| `grammar.js` | Upstream plus the documented Grepple patches | `a02819166621d20b5ce86d2c86c2aa1d6c3ed38d7dbe5d6e94b238044758b257` |
| `grammar.json` | Generated from patched `grammar.js` | `c6018451cf1d724e58c231a2e8a69b47fa40ffe08bc9ee79f68acfbf0140c10e` |
| `node-types.json` | Generated from patched `grammar.js` | `f4ef42e228a8d69e35bb7cb018774e907c1ea81ea6cefd811df4d2df6c37433c` |
| `parser.c` | Generated from patched `grammar.js` | `f9592cef68331c069553920c309635669afb7fe6dd6ff46b7895420c43ef8aaf` |
| `scanner.c` | `src/scanner.c` | `6d0af8aa49c59b08107c9e66f45010da7b53a1982a6ec10dfb26f3a4fe0c37f0` |
| `tree_sitter/alloc.h` | `src/tree_sitter/alloc.h` | `253b44a7b4313a7afd0c505c2fc6e7ce4b8e78955ebf4be3ea000532ec060673` |
| `tree_sitter/array.h` | `src/tree_sitter/array.h` | `4ff743903dc46f5db6aa54f31c6b4d160a8a9779e5b2ab1ee59ae7ebcd850ea1` |
| `tree_sitter/parser.h` | `src/tree_sitter/parser.h` | `a3eb18ef034b3f4255b965a26caa276f9cfe13a79573b402f1a12dc5018052aa` |
| `binding.go` | Grepple binding | `439072e40581c1e7648aaf0c6b97cfd6bb6628e1d99385c1cffdb6a69b5efb6d` |

## Verified regeneration

From `internal/gritql/internal/gritgrammar` in this checkout:

```sh
scratch=$(mktemp -d)
cp grammar.js "$scratch/grammar.js"
(cd "$scratch" && npx --yes tree-sitter-cli@0.22.6 generate)
sha256sum "$scratch/src/parser.c"
# f9592cef68331c069553920c309635669afb7fe6dd6ff46b7895420c43ef8aaf
```

This command was verified to reproduce the vendored patched `parser.c` exactly. The `src/` output is copied into this flat vendored runtime layout; generated package scaffolding is not retained. The `tree-sitter-cli` devDependency declared by upstream `package.json` is stale (`~0.21.0-pre-release-1`) and **does not** reproduce that parser hash. Version `0.22.6`, rather than the stale package declaration, is therefore the pinned regeneration tool. Regeneration is a maintainer provenance check only; Node.js is not needed to build or run Grepple.
