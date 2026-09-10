# File type support

## How files are classified

`grepple` searches text first and adds structural context when an application parser is available.

1. `search` discovers files, applies path/repository filters, reads text files, and matches lines.
2. Unreadable files and files containing a NUL byte are skipped; invalid UTF-8 is replaced before matching.
3. `parser.LanguageFor` classifies a filename by extension.
4. `parser.BuildSegments` uses tree-sitter for configured source languages, a heading scanner for Markdown, and one-line segments for plain text or parse failures.

Language detection is extension-based; MIME types and shebangs are not inspected. `--files` only reports discovered paths and does not read content, so it can include binary or unreadable files.

## Current support

| Language ID | Extensions | Parser |
| --- | --- | --- |
| `typescript` | `.ts` | tree-sitter TypeScript |
| `tsx` | `.tsx` | tree-sitter TSX |
| `javascript` | `.js`, `.jsx` | tree-sitter JavaScript |
| `go` | `.go` | tree-sitter Go |
| `python` | `.py`, `.pyi`, `.pyw` | tree-sitter Python |
| `java` | `.java` | tree-sitter Java |
| `kotlin` | `.kt`, `.kts` | tree-sitter Kotlin |
| `csharp` | `.cs` | tree-sitter C# |
| `c` | `.c`, `.h` | tree-sitter C |
| `cpp` | `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx` | tree-sitter C++ |
| `rust` | `.rs` | tree-sitter Rust |
| `shell` | `.sh`, `.bash`, `.zsh` | tree-sitter Bash |
| `markdown` | `.md`, `.markdown`, `.mdown`, `.mkd` | heading scanner |

Everything else remains searchable through the plain-text fallback. JSON (`.json`) and YAML (`.yaml`, `.yml`) additionally have lightweight outline support, but search segments use the plain-text fallback.

Structural source segments include comments attached immediately before a declaration, including JSDoc, Go documentation comments, Python comments, Rust doc comments, and equivalent forms in the other supported languages. One blank line is allowed between the comment and declaration. Attributes, annotations, and decorators between them are included; trailing comments attached to an earlier statement are not.

Every tree-sitter-backed language in the table supports local syntax-based navigation. `--related` reports bounded outgoing callees and potential callers, `--at PATH:LINE` retrieves a callable declaration by location, and `--follow-related N` expands up to two resolved callees per level under a shared 400-line budget. Confidence distinguishes qualified `exact`, explicit-import `import-resolved`, safely narrowed `context-resolved`, globally `unique-terminal`, and unresolved `candidate` matches; candidate text includes an explicit `--at PATH:LINE` suggestion. Go and TypeScript use imports, direct parameter/method-receiver types, source-ordered lexical bindings from typed declarations or constructor/composite literals, and same-file typed member chains before terminal fallback. Nested bindings remain confined to their branch or block. Resolution remains syntax-based rather than a type-checked call graph, so call-return inference, cross-file fields, and promoted methods can remain ambiguous. Go additionally recognizes interface methods and function-valued struct fields.

## Package boundaries

- `api` — dependency-free Grepple HTTP contract types and shared wire constants; it contains no validation, transport, persistence, or application logic.
- `parser/content.go` — language detection and parser-local content helpers.
- `parser/language.go` — source-language adapter interface, registry, and shared structural rules.
- `parser/language_<name>.go` — grammar selection, structural rules, and outline implementation for one source language.
- `parser/language_ecmascript.go` — outline helpers shared by JavaScript and TypeScript.
- `parser/tree_sitter.go` — parser pool and language-independent AST helpers.
- `parser/segments.go` — shared AST and plain-text structural segment construction.
- `parser/navigation.go` — language-neutral callable declaration and call extraction using each adapter's structural rules.
- `parser/markdown.go` and `parser/structured.go` — specialized non-tree-sitter parsing.
- `parser/outline.go` — public outline orchestration and shared symbol helpers.
- `search/search_engine.go` — discovery, filtering, reading, and line matching; it passes only content, language, and hit lines to parser.
- `search/related.go` — shared declaration indexing, syntax-based resolution, ranking, callers, and bounded recursive expansion for every navigable language.
- `search/result.go` — construction of `api.FileResult` values from internal matches and parser segments.

`api` must not import an application package. `parser` must not import `search` or `internal/cli`. Backend and CLI consumers import shared endpoint DTOs directly from `api`; search keeps resolved `Params`, validation, and internal file matches. The CLI may use parser directly for outlines.

## Adding a tree-sitter-backed file type

1. Add and pin a grammar dependency with Go bindings compatible with `github.com/tree-sitter/go-tree-sitter`.
2. Add a `languageAdapter` implementation in `parser/language_<name>.go`; keep its grammar, structural rules, and outline logic in that file.
3. Register the adapter in `parser/language.go` and map the exact supported extensions in `parser.LanguageFor`.
4. Add representative fixtures under `testdata/<language>/` covering declarations, containers, body matches, unrelated declarations, and language-specific syntax.
5. Extend `TestTreeSitterLanguageParity` in `parser/tree_sitter_test.go` and add exact segment/outline assertions where appropriate.
6. Run:

```bash
gofmt -w parser search
go test ./...
go vet ./...
```

Tree-sitter grammars use CGO, so builds require a C compiler. Do not map an extension until parsing, names, ranges, summaries, and fallback behavior are tested.
