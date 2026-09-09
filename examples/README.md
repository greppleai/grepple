# grepple examples

Runnable examples that show what `grepple` prints for different **file types** and
**options**. Every command below is run from **this `examples/` directory** against
[`sample-files/`](./sample-files) or [`advanced-files/`](./advanced-files), so you can reproduce each block exactly.

All output samples are kept inside fenced code blocks so they stay visually separated
from the surrounding prose.

## Setup

Build the binary once from the repo root, then run the examples from here:

```bash
make build                 # produces ./bin/grepple
export PATH="$PWD/bin:$PATH"
cd examples
```

## Fixtures

| File | Type | Structural support |
| --- | --- | --- |
| [`sample-files/server.go`](./sample-files/server.go) | Go | tree-sitter (funcs, methods, structs) |
| [`sample-files/Button.tsx`](./sample-files/Button.tsx) | TSX | tree-sitter (types, functions, JSX) |
| [`sample-files/pkg/widget.tsx`](./sample-files/pkg/widget.tsx) | TSX (nested) | used for glob/directory scoping |
| [`sample-files/Order.java`](./sample-files/Order.java) | Java | tree-sitter (classes, records, methods) |
| [`sample-files/Inventory.kt`](./sample-files/Inventory.kt) | Kotlin | tree-sitter (classes, functions) |
| [`sample-files/config.json`](./sample-files/config.json) | JSON | outline key/type tree |
| [`sample-files/values.yaml`](./sample-files/values.yaml) | YAML (larger) | outline key/type tree |
| [`sample-files/deployment.yaml`](./sample-files/deployment.yaml) | YAML (tiny) | dumped whole (outline > file) |
| [`sample-files/guide.md`](./sample-files/guide.md) | Markdown | heading-chain context |
| [`sample-files/server.log`](./sample-files/server.log) | Log / plain text | plain-line fallback |
| [`advanced-files/`](./advanced-files) | All baseline code languages | complex, executable parser regression fixtures |

## Guides

1. [Code search (structural context)](./01-code-search.md)
2. [Markdown & plain-text search](./02-markdown-and-text.md)
3. [Outlines (`--outline`)](./03-outline.md)
4. [Counting & listing probes](./04-counting-and-listing.md)
5. [Matching options & flags](./05-options-and-flags.md)
6. [JSON output & paging](./06-json-and-paging.md)
7. [Advanced language fixtures and regression tests](./07-advanced-language-fixtures.md)

> Line numbers, timestamps, and paths in the samples come straight from the fixtures.
> If you edit a fixture, re-run the command to refresh the block.
