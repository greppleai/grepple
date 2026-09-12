# Vendored tree-sitter GritQL grammar provenance

- Upstream URL: <https://github.com/getgrit/tree-sitter-gritql.git>
- Commit: `f9d98660bd7ae78c9211cb52e295bcd6531a8121`
- Upstream relationship: derived from the exact gitlink used by `getgrit/gritql` commit `4ca283484ab9bd11ca3cbc9e14b5ad2db5d61a37`.
- Grepple patch: `languageName` additionally accepts `javascript`, `typescript`, `tsx`, and `shell`; no query operators or runtime behavior were added to the vendored grammar.

## Files and SHA-256

All copied upstream files are covered by the exact vendored `LICENSE` (MIT, copyright 2024 Iuvo AI, Inc.). `binding.go` is the Grepple Go binding and is covered by the Grepple project license.

| File | Origin | SHA-256 |
| --- | --- | --- |
| `LICENSE` | `LICENSE` | `efa8bfef33f67df331a760fbadae76841d5ccab4135fa5c53e44b5b3253d55cb` |
| `grammar.js` | Upstream plus the documented Grepple language-name patch | `4cd04e80b11eb64a191afacc1d35d1db18c3311265417fe7446db4d8999d875b` |
| `grammar.json` | Generated from patched `grammar.js` | `7fccb5ce9c8499bff599a3b146b0197e044b5c21ae8d85f814f5b7b86dd6bfea` |
| `node-types.json` | Generated from patched `grammar.js` | `7f5b2ea5b69bdc7ebdfbe32a019d7a1153006ef6ef5b1e9bcfbcfe9dad0b056a` |
| `parser.c` | Generated from patched `grammar.js` | `9c2f12ca89c46dfb4d404dd74c08cbbf5fcf3f9ac5e0bddf32f7d1193e4f47a3` |
| `scanner.c` | `src/scanner.c` | `6d0af8aa49c59b08107c9e66f45010da7b53a1982a6ec10dfb26f3a4fe0c37f0` |
| `tree_sitter/alloc.h` | `src/tree_sitter/alloc.h` | `253b44a7b4313a7afd0c505c2fc6e7ce4b8e78955ebf4be3ea000532ec060673` |
| `tree_sitter/array.h` | `src/tree_sitter/array.h` | `4ff743903dc46f5db6aa54f31c6b4d160a8a9779e5b2ab1ee59ae7ebcd850ea1` |
| `tree_sitter/parser.h` | `src/tree_sitter/parser.h` | `a3eb18ef034b3f4255b965a26caa276f9cfe13a79573b402f1a12dc5018052aa` |
| `binding.go` | Grepple binding | `439072e40581c1e7648aaf0c6b97cfd6bb6628e1d99385c1cffdb6a69b5efb6d` |

## Verified regeneration

From `gritql/internal/gritgrammar` in this checkout:

```sh
npx --yes tree-sitter-cli@0.22.6 generate
sha256sum src/parser.c
# 9c2f12ca89c46dfb4d404dd74c08cbbf5fcf3f9ac5e0bddf32f7d1193e4f47a3
```

This command was verified to reproduce the vendored patched `parser.c` exactly. The `src/` output is copied into this flat vendored runtime layout; generated package scaffolding is not retained. The `tree-sitter-cli` devDependency declared by upstream `package.json` is stale (`~0.21.0-pre-release-1`) and **does not** reproduce that parser hash. Version `0.22.6`, rather than the stale package declaration, is therefore the pinned regeneration tool. Regeneration is a maintainer provenance check only; Node.js is not needed to build or run Grepple.
