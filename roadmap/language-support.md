# Language support roadmap

Grepple searches readable non-NUL text without a parser. Tree-sitter adds syntax-aware segments and outlines; navigation and resolution vary by language and are not type checking. For the generated feature matrix, extensions, and precise limitations, see [file type support](../docs/file-type-support.md) or run `grepple languages` (`--json` for automation).

## Supported languages

- Go (`.go`)
- JavaScript/JSX (`.js`, `.jsx`) and TypeScript/TSX (`.ts`, `.mts`, `.cts`, `.tsx`)
- Python (`.py`, `.pyi`, `.pyw`), Java (`.java`), Kotlin (`.kt`, `.kts`), Dart (`.dart`), Swift (`.swift`), C# (`.cs`)
- C (`.c`, `.h`), C++ (`.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx`), Rust (`.rs`), PHP (`.php`), Shell (`.sh`, `.bash`, `.zsh`)
- CSS (`.css`), Svelte (`.svelte`), and Terraform/HCL (`.tf`, `.tfvars`, `.hcl`, `.tfbackend`)

These have Tree-sitter-backed structural context and outlines, but **not identical navigation features**. Callable search and graph navigation are syntax-based for code; CSS exposes configuration declarations rather than calls, HCL exposes literal top-level blocks and function invocations but not resource references, and Svelte navigation is limited to supported embedded scripts and styles. Swift call resolution is same-file only. Unsupported or ambiguous targets must stay unresolved. Type-checked dispatch, inheritance/overload selection, dynamic calls, and cross-language FFI are outside the current baseline.

Markdown has heading-aware segments and outlines, not code navigation. JSON/YAML have lightweight shape outlines and plain-text search segments; `.tf.json` is not parsed as HCL. The generated matrix, not this summary, is authoritative for individual features. Cross-feature capability output (`grepple languages`, `--json`, `--markdown`) and registration/parity tests are already in place; do not duplicate the language registry to extend them.

## Harden existing languages

Representative tests and generic benchmarks exist, but these are still open coverage or policy decisions:

- [ ] Broaden golden fixtures for **each** supported adapter, including malformed and partially written files; current segment goldens cover only a subset.
- [ ] Add per-language cases for nested declarations, decorators/annotations, generics, and multiline signatures where applicable.
- [ ] Decide whether `.h` remains deterministically C (the current behavior) or needs evidence-backed repository/content context for C++ headers.
- [ ] Detect extensionless shell scripts by shebang without misclassifying arbitrary text; classification is currently extension-based.
- [ ] Benchmark parse and segment paths per language on representative files and set regression thresholds; existing parser/segment benchmarks are not a per-language budget.
- [ ] Gate grammar upgrades on language parity, outline/segment goldens, malformed-source behavior, and the parse-once invariant. `make schema-check` already checks generated grammar metadata but is not a substitute for those regressions.

## Terraform/HCL references

Currently the HCL graph records literal top-level declarations and HCL function calls, not links from traversals such as `aws_instance.web.id`, `var.ami`, `local.name`, or `module.app.output`. Keep traversals distinct from function calls and avoid inferring Terraform evaluation semantics.

- [ ] Index literal resource, data, variable, locals, and module declarations across `.tf` files in the same Terraform directory; retain source ranges and duplicate/ambiguous addresses.
- [ ] Resolve unambiguous static resource/data addresses, `var.<name>`, `local.<name>`, and the `module.<name>` prefix to local declarations. Leave computed traversals and module output targets unresolved.
- [ ] Test cross-file resolved, missing, ambiguous, and unsupported references, including range evidence and separation from function calls; update the capability documentation.
- [ ] Investigate module sources/outputs, providers/aliases, `count`/`for_each`, and dynamic/indexed addresses separately. Require evidence for module boundaries and workspace context rather than guessing.

## New languages (demand-driven, not ranked)

Ruby, Scala, Protocol Buffers, SQL (after a dialect policy), Lua, Elixir, Objective-C, R, Clojure, Groovy, Perl, and Zig remain candidates, **not committed implementation tasks**.

For any new Tree-sitter language: pin compatible Go grammar bindings; add an owning `languageAdapter` and tested extension mapping; define syntax and navigation policies; implement useful outlines and honest capability reporting; test detection, segments, navigation, malformed input, and parse-once behavior; document ambiguity and unsupported constructs. Do not advertise a feature before its implementation and parity tests exist.
