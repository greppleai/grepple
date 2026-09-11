# Vendored tree-sitter GritQL grammar provenance

- Upstream URL: <https://github.com/getgrit/tree-sitter-gritql.git>
- Commit: `f9d98660bd7ae78c9211cb52e295bcd6531a8121`
- Package version: `0.1.0`
- Upstream relationship: this is the exact gitlink used by `getgrit/gritql` commit `4ca283484ab9bd11ca3cbc9e14b5ad2db5d61a37`.
- Runtime contents: generated C syntax data and its Go/cgo binding only. Node.js and Rust are not runtime or build dependencies.

## Files and SHA-256

All copied upstream files are covered by the exact vendored `LICENSE` (MIT, copyright 2024 Iuvo AI, Inc.). `binding.go` is the Grepple Go binding and is covered by the Grepple project license.

| File | Origin | SHA-256 |
| --- | --- | --- |
| `LICENSE` | `LICENSE` | `efa8bfef33f67df331a760fbadae76841d5ccab4135fa5c53e44b5b3253d55cb` |
| `grammar.js` | `grammar.js` | `9d3093779a6d63887272cb41ba102ba706e2a0b1f9d12a33bb207c6bb9d51aa3` |
| `grammar.json` | `src/grammar.json` | `ba69134a74ae044c1971e4dcab631ca224f23ba4675ddff106226b639d66a6d5` |
| `node-types.json` | `src/node-types.json` | `5d674b485c87ca5a14497c6aff876693f34a8b5886f8fa10f11abb513ee6e15c` |
| `parser.c` | `src/parser.c` | `ecd35eea003350e68c8a26da3872a46c7681c091aa597389bddd72285ad640dc` |
| `scanner.c` | `src/scanner.c` | `6d0af8aa49c59b08107c9e66f45010da7b53a1982a6ec10dfb26f3a4fe0c37f0` |
| `tree_sitter/alloc.h` | `src/tree_sitter/alloc.h` | `253b44a7b4313a7afd0c505c2fc6e7ce4b8e78955ebf4be3ea000532ec060673` |
| `tree_sitter/array.h` | `src/tree_sitter/array.h` | `4ff743903dc46f5db6aa54f31c6b4d160a8a9779e5b2ab1ee59ae7ebcd850ea1` |
| `tree_sitter/parser.h` | `src/tree_sitter/parser.h` | `a3eb18ef034b3f4255b965a26caa276f9cfe13a79573b402f1a12dc5018052aa` |
| `binding.go` | Grepple binding | `439072e40581c1e7648aaf0c6b97cfd6bb6628e1d99385c1cffdb6a69b5efb6d` |

## Verified regeneration

From a clean checkout at the commit above:

```sh
npx --yes tree-sitter-cli@0.22.6 generate
sha256sum src/parser.c
# ecd35eea003350e68c8a26da3872a46c7681c091aa597389bddd72285ad640dc
```

This command was verified to reproduce the vendored `parser.c` exactly. The `tree-sitter-cli` devDependency declared by upstream `package.json` is stale (`~0.21.0-pre-release-1`) and **does not** reproduce that parser hash. Version `0.22.6`, rather than the stale package declaration, is therefore the pinned regeneration tool. Regeneration is a maintainer provenance check only; Node.js is not needed to build or run Grepple.
