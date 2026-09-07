# File type support

## How files are classified

`grepple` searches text first and adds structural context when an application parser is available.

1. `internal/search` discovers files, applies path/repository filters, reads text files, and matches lines.
2. Unreadable files and files containing a NUL byte are skipped; invalid UTF-8 is replaced before matching.
3. `internal/parser.LanguageFor` classifies a filename by extension.
4. `internal/parser.BuildSegments` uses tree-sitter for configured source languages, a heading scanner for Markdown, and one-line segments for plain text or parse failures.

Language detection is extension-based; MIME types and shebangs are not inspected. `--files` only reports discovered paths and does not read content, so it can include binary or unreadable files.

## Current support

| Language ID | Extensions | Parser |
| --- | --- | --- |
| `typescript` | `.ts` | tree-sitter TypeScript |
| `tsx` | `.tsx` | tree-sitter TSX |
| `javascript` | `.js`, `.jsx` | tree-sitter JavaScript |
| `go` | `.go` | tree-sitter Go |
| `java` | `.java` | tree-sitter Java |
| `kotlin` | `.kt`, `.kts` | tree-sitter Kotlin |
| `markdown` | `.md`, `.markdown`, `.mdown`, `.mkd` | heading scanner |

Everything else remains searchable through the plain-text fallback. JSON (`.json`) and YAML (`.yaml`, `.yml`) additionally have lightweight outline support, but search segments use the plain-text fallback.

## Package boundaries

- `internal/api` — dependency-free Grepple HTTP contract types and shared wire constants; it contains no validation, transport, persistence, or application logic.
- `internal/parser/content.go` — language detection and parser-local content helpers.
- `internal/parser/tree_sitter.go` — grammar imports, language configuration, parser pool, and AST helpers.
- `internal/parser/segments.go` — AST and plain-text structural segments.
- `internal/parser/outline.go` — source, Markdown, JSON, and YAML outlines.
- `internal/search/search_engine.go` — discovery, filtering, reading, and line matching; it passes only content, language, and hit lines to parser.
- `internal/search/result.go` — construction of `api.FileResult` values from internal matches and parser segments.

`internal/api` must not import an application package. `internal/parser` must not import `internal/search` or `internal/grepplecli`. Router, shard, CLI, repository, and search consumers import shared endpoint DTOs directly from `internal/api`; search keeps resolved `Params`, validation, and internal file matches. The CLI may use parser directly for outlines.

## Adding a tree-sitter-backed file type

1. Add and pin a grammar dependency with Go bindings compatible with `github.com/tree-sitter/go-tree-sitter`.
2. Import the grammar only in `internal/parser/tree_sitter.go` and add a `languageConfig` entry. Configure the smallest useful sets of structural, context, container, body, function, and name node kinds.
3. Map the exact supported extensions in `internal/parser.LanguageFor`.
4. Add representative fixtures under `testdata/<language>/` covering declarations, containers, body matches, unrelated declarations, and language-specific syntax.
5. Extend `TestTreeSitterLanguageParity` in `internal/parser/tree_sitter_test.go` and add exact segment/outline assertions where appropriate.
6. Run:

```bash
gofmt -w internal/parser internal/search
go test ./...
go vet ./...
```

Tree-sitter grammars use CGO, so builds require a C compiler. Do not map an extension until parsing, names, ranges, summaries, and fallback behavior are tested.
