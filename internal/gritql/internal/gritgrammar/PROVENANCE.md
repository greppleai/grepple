# Vendored tree-sitter GritQL grammar provenance

- Upstream URL: <https://github.com/getgrit/tree-sitter-gritql.git>
- Commit: `f9d98660bd7ae78c9211cb52e295bcd6531a8121`
- Upstream relationship: derived from the exact gitlink used by `getgrit/gritql` commit `4ca283484ab9bd11ca3cbc9e14b5ad2db5d61a37`.
- Grepple patch: `languageName` additionally accepts `javascript`, `typescript`, `tsx`, `shell`, `dart`, `swift`, and `svelte`; `predicateMatch` additionally accepts `emptyPredicate` as a direct right-hand side; `_pattern` additionally accepts `patternParent` for the direct `parent kind("function")` test. The evaluator implements both predicates without an external runtime.

## Files and SHA-256

All copied upstream files are covered by the exact vendored `LICENSE` (MIT, copyright 2024 Iuvo AI, Inc.). `binding.go` is the Grepple Go binding and is covered by the Grepple project license.

| File | Origin | SHA-256 |
| --- | --- | --- |
| `LICENSE` | `LICENSE` | `efa8bfef33f67df331a760fbadae76841d5ccab4135fa5c53e44b5b3253d55cb` |
| `grammar.js` | Upstream plus the documented Grepple patches | `ffb946e4461845fcb774f0a240c34ffe5dd02b74ca228c91afd29460d63c5378` |
| `grammar.json` | Generated from patched `grammar.js` | `5c28096821ccd984be27b157af5e7127a623d004128142059914314d6810bdcb` |
| `node-types.json` | Generated from patched `grammar.js` | `d0d46fb6463e5be6cbb9404680d57e23e12585f67225cc49945b70a41916df37` |
| `parser.c` | Generated from patched `grammar.js` | `30a928a38e41c0380bc7185f048f4aad76275f554916fa24cd9201017c2f8262` |
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
# 30a928a38e41c0380bc7185f048f4aad76275f554916fa24cd9201017c2f8262
```

This command was verified to reproduce the vendored patched `parser.c` exactly. The `src/` output is copied into this flat vendored runtime layout; generated package scaffolding is not retained. The `tree-sitter-cli` devDependency declared by upstream `package.json` is stale (`~0.21.0-pre-release-1`) and **does not** reproduce that parser hash. Version `0.22.6`, rather than the stale package declaration, is therefore the pinned regeneration tool. Regeneration is a maintainer provenance check only; Node.js is not needed to build or run Grepple.
