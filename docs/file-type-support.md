# File type support

## How files are classified

`grepple` searches text first and adds structural context when an application parser is available.

1. `search` discovers files, applies path/repository filters, reads text files, and matches lines.
2. Unreadable files and files containing a NUL byte are skipped; invalid UTF-8 is replaced before matching.
3. `parser.LanguageFor` classifies a filename by extension.
4. `parser.BuildSegments` uses tree-sitter for configured source languages, a heading scanner for Markdown, and one-line segments for plain text or parse failures.

Language detection is extension-based; MIME types and shebangs are not inspected. `--files` only reports discovered paths and does not read content, so it can include binary or unreadable files.

## Current support matrix

Status: ✅ production implementation; 🟡 production but specialized or intentionally limited (see notes); ❌ not implemented; 🧪 experimental. There are currently no 🧪 runtime entries—the packed/native-tree experiments do not back shipped features.

`grepple` text matching covers every readable non-NUL file. Per-file `--count`, complete paging-independent `--count-summary`, and matching itself do not require a parser. `--line-only` also works without parsing; for parser-backed source it additionally reports `PATH:START-END` when a matching line begins a multi-line syntax construct, and falls back to `PATH:LINE` otherwise. Opt-in `--enclosing` reports body matches as `PATH:MATCH@START-END` using the nearest multi-line named syntax scope. Structural grep means the enclosing syntax or heading context used by default human search output. A source parse failure safely falls back to plain matching, but that fallback does not count as structural support.

<!-- grepple:language-matrix:start -->
| Language | Extensions | Text grep | Structural grep | Outline | Navigation | Focused structure | Focused flow | GritQL | Package bundle | Workspace bundle |
| --- | --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `c` | `.c`, `.h` | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ |
| `cpp` | `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx` | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ |
| `csharp` | `.cs` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| `go` | `.go` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `java` | `.java` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| `javascript` | `.js`, `.jsx` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| `json` | `.json` | ✅ | ❌ | 🟡 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `kotlin` | `.kt`, `.kts` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| `markdown` | `.md`, `.markdown`, `.mdown`, `.mkd` | ✅ | 🟡 | 🟡 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `python` | `.py`, `.pyi`, `.pyw` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| `rust` | `.rs` | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ |
| `shell` | `.sh`, `.bash`, `.zsh` | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ |
| `text` | any other extension | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `tsx` | `.tsx` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| `typescript` | `.ts`, `.mts`, `.cts` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ |
| `yaml` | `.yaml`, `.yml` | ✅ | ❌ | 🟡 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
<!-- grepple:language-matrix:end -->

The table is generated from parser, extraction, and GritQL registrations. `grepple languages` renders the terminal view, `grepple languages --json` emits the machine-readable matrix, and `grepple languages --markdown` regenerates the table above. Tests reject stale documentation.

Notes:

- JSX uses the canonical `javascript` parser language and JavaScript extraction adapter. TSX has its own parser language ID but intentionally shares the TypeScript declaration namespace and TypeScript-family extraction implementation.
- Markdown structural grep and outlines use the production lightweight heading scanner, not Tree-sitter. They provide heading hierarchy rather than application-code declarations.
- JSON and YAML use a production lightweight shape outline. Their grep results use plain-text segments, so structural grep is not implemented.
- Navigation is production and shared through `parser.NavigationGraph`, but remains syntax-based rather than type-checked. Confidence and candidate output expose unresolved ambiguity.
- GritQL support in this table means native read-only structural matching under the single production `gritql-v1` contract.

### Which implementation path is active

- **Text grep and discovery:** `search` is the default production path. It discovers files, applies ignore/path/repository filters, reads candidates, rejects NUL-containing files, normalizes invalid UTF-8 for matching, and matches text. Parser support is not required.
- **Structural grep context and outlines:** `parser.LanguageFor`, `parser.BuildSegments`, and `parser.BuildOutline` select parser-owned adapters. Go, TypeScript/TSX, JavaScript/JSX, Python, Java, Kotlin, C#, C, C++, Rust, and Shell all use this default Tree-sitter path. Markdown, JSON, and YAML use the lightweight paths shown above.
- **Navigation:** every Tree-sitter-backed language in the table uses `parser.NavigationGraph`; `search` resolves and renders that shared graph for `--at`, `--related`, and `--follow-related`. Typed field and export facts propagate safe cross-file receiver context. Go embedded fields support promoted-method candidates; TypeScript/TSX support inheritance, named/default import aliases, relative barrel re-exports, and nearest-`tsconfig.json` `baseUrl`/`paths` aliases. Resolution remains syntax-based, and ambiguous interface/overload/inheritance candidates remain explicit. This is the default implementation, not a legacy or experimental collector.
- **Focused architecture extraction:** Go, JavaScript/JSX, TypeScript/TSX, Python, Java, and Kotlin are registered in `extract`. They parse through `parser.Document` and reuse `parser.NavigationGraph`, while retaining production language-specific semantic enrichment and Mermaid rendering. Python covers classes, inheritance, annotated and unannotated attributes, decorators, and `.pyi` stubs. Java covers classes, interfaces, records, enums, inheritance, implementations, fields, constructors, and methods. Kotlin covers classes, interfaces, objects, data-class constructor properties, delegation-based inheritance, properties, and functions. Unsupported languages fail explicitly.
- **Native GritQL:** every Tree-sitter-backed language in the table is registered under the single production `gritql-v1` contract. All target source parses through `parser.Document`; the GritQL query grammar itself is a separate intentional parser. Language-local adapters define safe snippet wrappers and root categories, while query algebra, matching, limits, and diagnostics remain shared. Unsupported language declarations fail explicitly, with no text or alternate-language fallback.
- **Canonical package/workspace bundles:** remain production Go-only because their package, module, build-tag, route, and schema contracts are Go-specific. `grepple extract summary package|workspace` provides a bounded Markdown orientation projection from that same canonical IR. This is intentional rather than a temporary limitation.
- **Experimental code:** packed CST and native `TSTree` serialization exist only in benchmarks or opt-in build-tag experiments and do not back text search, structural context, outlines, navigation, extraction, or GritQL in the shipped CLI.

Structural source segments include comments attached immediately before a declaration, including JSDoc, Go documentation comments, Python comments, Rust doc comments, and equivalent forms in the other supported languages. One blank line is allowed between the comment and declaration. Attributes, annotations, and decorators between them are included; trailing comments attached to an earlier statement are not.

Every Tree-sitter-backed language in the table supports local syntax-based navigation. `--related` reports bounded outgoing callees and potential callers, `--at PATH:LINE` retrieves a callable declaration by location, and `--follow-related N` expands up to two resolved callees per level under a shared 400-line budget. If a caller/callee cap omits edges, human output reports the directional counts and complete JSON exposes them alongside the bounded points. Confidence distinguishes qualified `exact`, explicit-import `import-resolved`, safely narrowed `context-resolved`, globally `unique-terminal`, and unresolved `candidate` matches; candidate text includes an explicit `--at PATH:LINE` suggestion. Go and TypeScript use imports, direct parameter/method-receiver types, source-ordered lexical bindings, constructor/composite literals, local/imported return signatures, and package/module-aware cross-file typed member chains before terminal fallback. Nested bindings remain confined to their branch or block. Go additionally recognizes interface methods, function-valued struct fields, and embedded/promoted methods. TypeScript/TSX additionally recognize inheritance, re-exports, default/named aliases, and configured path aliases. These are conservative syntax facts rather than compiler dispatch; conflicts remain candidates.

`grepple graph callers`, `callees`, `dependencies`, `dependents`, and `impact` traverse the same resolved and candidate call edges. Exact symbol/location selectors choose one root; package, module, and root-path selectors choose a scope. Repeatable language and confidence filters apply before root selection and traversal. Traversal is deterministic, cycle-safe, bounded to depth 1–10 in the CLI, complete in JSON, and byte-bounded in compact output. Positional paths remain the larger graph universe.

`grepple boundaries [PATH...]` combines file-owned workflow patterns with concrete type spread. Workflow candidates retain callable co-usage and ordered sequences repeated across two or more external files; receiver-qualified member facts enrich combinations when ownership is known. Type candidates retain import-qualified concrete types and unambiguously owned project types spanning two or more files, with role, public-exposure, and production/test evidence. Both analyses use the shared navigation facts available for each language without fabricating unsupported type ownership.

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
