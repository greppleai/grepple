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
| Language | Extensions | Text grep | Structural grep | Outline | Navigation | Focused structure | Focused flow | GritQL | Directory architecture | Import relations | Entrypoints |
| --- | --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `c` | `.c`, `.h` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `cpp` | `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `csharp` | `.cs` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `go` | `.go` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `java` | `.java` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `javascript` | `.js`, `.jsx` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `json` | `.json` | ✅ | ❌ | 🟡 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `kotlin` | `.kt`, `.kts` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `markdown` | `.md`, `.markdown`, `.mdown`, `.mkd` | ✅ | 🟡 | 🟡 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `python` | `.py`, `.pyi`, `.pyw` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `rust` | `.rs` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `shell` | `.sh`, `.bash`, `.zsh` | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ |
| `text` | any other extension | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `tsx` | `.tsx` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `typescript` | `.ts`, `.mts`, `.cts` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `yaml` | `.yaml`, `.yml` | ✅ | ❌ | 🟡 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

### Navigation fact support

| Language | Declarations | Calls | Imports | Type references | Fields | Member access | Entrypoints |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `c` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `cpp` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `csharp` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `go` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `java` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `javascript` | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ✅ |
| `json` | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `kotlin` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `markdown` | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `python` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `rust` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `shell` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `text` | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `tsx` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `typescript` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `yaml` | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
<!-- grepple:language-matrix:end -->

The tables are generated from parser, focused-extraction, GritQL, directory-architecture, and adapter-owned navigation registrations. `grepple languages` renders both terminal views, `grepple languages --json` emits the machine-readable matrix including `navigationFacts`, and `grepple languages --markdown` regenerates the tables above. Tests reject stale documentation.

A ✅ in the navigation-fact table means the parser adapter has a tested contract to emit that normalized `parser.NavigationGraph` fact kind. It does not promise that every language construct is modeled or that a repository target resolves. Emitted imports, calls, type references, and member accesses may remain ambiguous or unresolved and retain that uncertainty in graph output. A ❌ means the normalized fact kind is unsupported for that adapter; focused extraction may still project analogous language-specific structure and must not be mistaken for a navigation fact.

Notes:

- JSX uses the canonical `javascript` parser language and JavaScript extraction adapter. TSX has its own parser language ID but intentionally shares the TypeScript declaration namespace and TypeScript-family extraction implementation.
- Markdown structural grep and outlines use the production lightweight heading scanner, not Tree-sitter. They provide heading hierarchy rather than application-code declarations.
- JSON and YAML use a production lightweight shape outline. Their grep results use plain-text segments, so structural grep is not implemented.
- Navigation is production and shared through `parser.NavigationGraph`, but remains syntax-based rather than type-checked. Confidence and candidate output expose unresolved ambiguity.
- GritQL support in this table means native read-only structural matching under the single production `gritql-v1` contract.

### Which implementation path is active

- **Text grep and discovery:** `search` is the default production path. It discovers files, applies ignore/path/repository filters, reads candidates, rejects NUL-containing files, normalizes invalid UTF-8 for matching, and matches text. Parser support is not required.
- **Structural grep context and outlines:** `parser.LanguageFor`, `parser.BuildSegments`, and `parser.BuildOutline` select parser-owned adapters. Go, TypeScript/TSX, JavaScript/JSX, Python, Java, Kotlin, C#, C, C++, Rust, and Shell all use this default Tree-sitter path. Markdown, JSON, and YAML use the lightweight paths shown above.
- **Navigation:** every Tree-sitter-backed language in the table uses `parser.NavigationGraph`; `search` resolves and renders that shared graph for `--at`, `--related`, and `--follow-related`. Typed field and export facts propagate safe cross-file receiver context. Go embedded fields support promoted-method candidates; TypeScript/TSX support inheritance, named/default import aliases, relative barrel re-exports, and nearest-`tsconfig.json` `baseUrl`/`paths` aliases. Python records module-level `import` and `from ... import ...` aliases and resolves absolute or explicit relative modules against the selected source universe while preserving multiple matching source roots as ambiguity; function-local imports remain unsupported rather than being promoted into incorrect file-wide bindings. Java records package, type, wildcard, and static imports; Kotlin records package, declaration, wildcard, and aliased imports. JVM targets resolve through adapter-owned package/export facts rather than source-path conventions. C# records unambiguous file-scoped or single block namespaces plus compilation-unit and namespace-local `using`, namespace-or-type aliases, `using static`, and `global using` directives; files containing multiple block namespaces remain unscoped instead of being merged. Because C# alias syntax does not distinguish namespace aliases from type aliases, aliases remain import evidence and are not promoted into call bindings. Global usings are recorded at their declaration site but are not propagated to other files without project resolution. Rust records file-module `use` trees, groups, aliases, globs, visibility-aware re-exports, conventional external `mod` declarations, inline-module scopes, and directly specified normal or raw string-literal `#[path]` targets. Rust resolution starts only from selected `src/lib.rs`, `src/main.rs`, and `src/bin` roots, traverses syntax-evidenced module edges, resolves `crate`, `self`, and `super` paths, preserves crate and inline ownership, and enforces private, `pub`, `pub(crate)`, `pub(super)`, `pub(self)`, and ancestor-valid `pub(in path)` access for modules, imports, and calls. Bare external paths and conditional or unparseable path attributes remain unresolved rather than guessed. Resolution remains syntax-based, and ambiguous interface/overload/inheritance/import candidates remain explicit. This is the default implementation, not a legacy or experimental collector.
- **Focused architecture extraction:** Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, C#, Rust, C, and C++ are registered in `extract`. They parse through `parser.Document` and reuse `parser.NavigationGraph`, while retaining adapter-owned semantic enrichment and Mermaid rendering. C and C++ conservatively project named structs, unions, classes, enums, typedef-wrapped aggregate aliases, fields, variants, direct C++ base classes, free functions, methods, access visibility, static members, graph-backed calls, and global `main` process entrypoints. Overloads remain ambiguous; templates, macro expansion, conditional compilation, linkage/ABI, build-system ownership, and cross-file namespace merging are not inferred. Rust projects structs, tuple structs, enums, traits, nested inline-module ownership, fields, variants, associated types, methods, visibility, trait implementations, graph-backed calls, unambiguous same-crate cross-file `impl` blocks resolved through explicit qualified paths or module-level imports, and top-level `main` process entrypoints only in selected `src/main.rs` and `src/bin` crate roots. Rust declarations, modules, imports, calls, and implementation attachment preserve and enforce private, `pub`, `pub(crate)`, `pub(super)`, `pub(self)`, and ancestor-valid `pub(in path)` access. Cargo dependencies, conditional `cfg` ownership, and framework APIs remain uninterpreted. Unsupported focused languages fail explicitly.
- **Process entrypoints:** adapters emit only syntax-evidenced callable entrypoints. JavaScript and TypeScript/TSX accept one uniquely declared top-level function only when a top-level strict `require.main === module` CommonJS guard directly calls it; reversed operands, exported declarations, and block or direct consequences are accepted. Naming alone, loose equality, indirect calls, nested or duplicate declarations, and explicit `.mts` ESM sources remain unclassified; package-manifest `bin` mapping and build-output inference are not yet modeled. Python accepts one uniquely declared synchronous top-level function in a runtime source module only when a top-level `if __name__ == "__main__"` guard directly calls that function; reversed operands and either ordinary quote style are accepted, while `.pyi` stubs, naming alone, indirect calls, methods, duplicate definitions, async calls without `await`, and other conditions remain unclassified. Java accepts conventional `public static void main(String[]|String...)` methods. Kotlin accepts public top-level `.kt` `main` functions with no parameters or `Array<String>` and a syntax-evidenced `Unit` result; scripts, members, renamed functions, and return types requiring inference remain unclassified. C# accepts non-generic static `Main` methods returning `void`, `int`, or an explicitly qualified `System.Threading.Tasks.Task` form, with no parameters or one value `string[]` parameter; top-level statements, unqualified task types, and project-selected startup objects remain unclassified. Go, C/C++, and Rust use the conservative rules described above.
- **C/C++ evidence limits:** Direct quoted and angle-bracket include syntax is retained as import evidence. Only quoted includes whose source-relative normalized path exactly matches a selected file are resolved. Angle-bracket includes, macro-computed paths, basename matches outside the source directory, compiler include directories, and build-system search paths remain unresolved rather than guessed. Normalized navigation field facts require a directly named aggregate owner; anonymous C aggregates wrapped by a typedef remain without a field owner rather than borrowing the alias, while focused structure extraction may still project that typedef aggregate.
- **Field evidence limits:** JavaScript class fields remain unsupported as normalized field facts because the JavaScript grammar provides no syntax-level type annotation; initializer and JSDoc inference are intentionally excluded. TypeScript/TSX retain their explicit type syntax. Python fields require an annotated assignment directly in a class body; method-local annotations and inferred `self` assignments are not promoted. Kotlin fields require an explicitly typed `val` or `var` constructor property or a directly declared, explicitly typed class-body property. C# fields require an explicit field or property type. Nested declarations remain attached only to their nearest syntax-evidenced owner.
- **Native GritQL:** every Tree-sitter-backed language in the table is registered under the single production `gritql-v1` contract. All target source parses through `parser.Document`; the GritQL query grammar itself is a separate intentional parser. Language-local adapters define safe snippet wrappers and root categories, while query algebra, matching, limits, and diagnostics remain shared. Unsupported language declarations fail explicitly, with no text or alternate-language fallback.
- **Directory architecture:** every Tree-sitter-backed language contributes files, outlines, declarations, and strongly resolved calls through `grepple architecture directory|resolve|why`; Go, JavaScript/TypeScript, Python, Java, Kotlin, C#, and Rust adapters additionally contribute source-linked import relations. Go, JavaScript/TypeScript, Python, Java, Kotlin, and Rust can also contribute imported-type relations from normalized type facts. C# emits parameter-type facts but conservatively leaves namespace-or-type aliases unqualified rather than promoting them into type relations; C/C++ type facts likewise remain unqualified unless future explicit compiler/build evidence binds an included symbol. Coverage names unsupported, unqualified, ambiguous, and unresolved relation evidence. Physical directories are not presented as package/module/layer semantics, and unknown visibility remains explicit.
- **Experimental code:** packed CST and native `TSTree` serialization exist only in benchmarks or opt-in build-tag experiments and do not back text search, structural context, outlines, navigation, extraction, or GritQL in the shipped CLI.

Structural source segments include comments attached immediately before a declaration, including JSDoc, Go documentation comments, Python comments, Rust doc comments, and equivalent forms in the other supported languages. One blank line is allowed between the comment and declaration. Attributes, annotations, and decorators between them are included; trailing comments attached to an earlier statement are not.

Every Tree-sitter-backed language in the table supports local syntax-based navigation. Structural search defaults to related navigation and one level of bounded caller/callee previews; `--no-related` disables it. Navigation reports source-declared types used by the matched callable, outgoing callees, and potential callers. Type points identify parameter, receiver, local, or result roles; human output emits each complete bounded declaration once in a final source-range-deduplicated appendix whose `HASH│LINE│content` rows can be edited directly. Local navigation resolves from the nearest `grepple.json`, `.git`, or `go.work` repository/workspace root even when text-search paths are narrower, while an explicit root remains authoritative. `--at PATH:LINE` retrieves a callable declaration by location, and `--follow-related N` expands up to two resolved callers and two resolved callees per level under a shared 400-line budget. If a caller, callee, or type cap omits evidence, human and JSON output report the directional counts. Confidence distinguishes qualified `exact`, explicit-import `import-resolved`, safely narrowed `context-resolved`, globally `unique-terminal`, exact server artifact `dependency-resolved`, explicit `dependency-unresolved`, and unresolved `candidate` matches; candidate text includes an explicit `--at PATH:LINE` suggestion. For local and remote searches, the CLI/server qualifies imported references from exact ecosystem evidence and asks the configured resolver for a matching indexed artifact: Go uses `go.mod`/`go.sum` plus versioned replacements and `go.work`; JavaScript/TypeScript uses direct `package.json` dependencies locked by `npm-shrinkwrap.json` or `package-lock.json` versions 1–3, with shrinkwrap taking npm precedence; Rust uses registry dependencies from `Cargo.toml` locked by `Cargo.lock`; Java and Kotlin use exact direct Maven `pom.xml` coordinates and preserve multiple coordinate candidates until indexed package declarations disambiguate them. Malformed npm locks and locks with missing or unsupported versions remain non-authoritative; an invalid selected shrinkwrap does not fall back to `package-lock.json`. Yarn classic/Berry, pnpm, Bun, and Corepack-selected managers are not yet dependency qualifiers and are never parsed as npm package locks. Local replacements, npm file/link dependencies, Cargo path/git dependencies, unresolved Maven properties, unavailable servers, and missing or wrong-version artifacts remain unresolved without changing local graph output. Gradle and Maven dependency-management/BOM resolution are not yet dependency qualifiers. Go semantic-import-version paths such as `github.com/gofiber/fiber/v3` retain the source package qualifier, so `fiber.Ctx` and methods on `*fiber.App` remain exact external evidence instead of falling through to an unrelated same-terminal declaration. Go, JavaScript/TypeScript, Java/Kotlin, C#, and Rust reject local terminal-name fallback when syntax identifies an unresolved external import; Python retains ambiguous local import roots as candidates while already preserving missing external imports as unresolved. Adapters marked for type-reference facts feed syntax-evidenced callable parameter and result types into navigation while their outlines supply complete source-declared type ranges. Go and TypeScript additionally use source-ordered lexical bindings, constructor/composite literals, local/imported return signatures, and package/module-aware cross-file typed member chains before terminal fallback. Nested bindings remain confined to their branch or block. Go additionally recognizes interface methods, function-valued struct fields, and embedded/promoted methods. TypeScript/TSX additionally recognize inheritance, re-exports, default/named aliases, and configured path aliases. These are conservative syntax facts rather than compiler dispatch; conflicts remain candidates.

For npm aliases such as `widget-alias: npm:@acme/widgets@^2`, navigation preserves the source import path as `widget-alias`, verifies the lock entry names `@acme/widgets`, and qualifies the resolved module/package path as `@acme/widgets`. Version 1 legacy alias entries derive both the package name and exact selected version from their locked `npm:` value; mismatched manifest and lock identities remain unresolved.

`grepple graph callers`, `callees`, `dependencies`, `dependents`, and `impact` traverse the same resolved and candidate navigation edges. `dependencies` means outgoing call/navigation edges and `dependents` means incoming call/navigation edges; neither command describes a package-manager, module, or build-system dependency graph. Exact symbol/location selectors choose one root; package, module, and root-path selectors choose a scope. Repeatable language and confidence filters apply before root selection and traversal. Traversal is deterministic, cycle-safe, bounded to depth 1–10 in the CLI, complete in JSON, and byte-bounded in compact output. Positional paths remain the larger graph universe.

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
