# `gritql-go-v1` compatibility contract

`gritql-go-v1` is Grepple's closed, Go-native, read-only detection subset of GritQL. It is not an alias for any upstream GritQL release. A conforming implementation accepts exactly the syntax below and rejects every other construct; it does not invoke an external engine, Node, a shell, or a fallback interpreter.

## 1. Closed syntax

The grammar is EBNF. Literal words and punctuation are quoted. `EOF` means the end of the pattern body.

```ebnf
pattern          = spacing, language, line_end, spacing, query, spacing, EOF ;
language         = "language", hspace1, "go" ;

query            = prefix, [ spacing, where_clause ] ;
prefix           = snippet
                 | regex
                 | "and", req_sep, query_block
                 | "or", req_sep, query_block
                 | unary, req_sep, prefix ;
unary            = "not" | "maybe" | "contains" | "within" ;

query_block      = "{", spacing, query, spacing, ",", spacing,
                   query, { spacing, ",", spacing, query },
                   [ spacing, "," ], spacing, "}" ;
where_clause     = "where", req_sep, constraint_block ;
constraint_block = "{", spacing, constraint,
                   { spacing, ",", spacing, constraint },
                   [ spacing, "," ], spacing, "}" ;
constraint       = metavariable, spacing, "<:", spacing, query ;

snippet          = "`", { snippet_char | snippet_escape | snippet_newline }, "`" ;
snippet_escape   = "\\`" | "\\\\" ;
snippet_newline  = "\n" | "\r\n" ;
snippet_char     = ? any Unicode scalar value except "`", "\\", CR, or LF ? ;
regex            = "r\"", { regex_char | regex_escape }, "\"" ;
regex_escape     = "\\\"" | "\\\\" | "\\n" | "\\r" | "\\t"
                 | "\\x", hex, hex | "\\u", hex, hex, hex, hex ;
regex_char       = ? any Unicode scalar value except '"', "\\", CR, or LF ? ;
hex              = "0"…"9" | "a"…"f" | "A"…"F" ;

metavariable     = "$_" | "$", meta_start, { meta_continue } ;
meta_start       = "A"…"Z" | "a"…"z" ;
meta_continue    = meta_start | "0"…"9" | "_" ;

spacing          = { whitespace | comment } ;
req_sep          = whitespace, spacing | comment, spacing ;
hspace1          = ( " " | "\t" ), { " " | "\t" } ;
line_end         = { " " | "\t" }, [ line_comment ], ( "\n" | "\r\n" ) ;
whitespace       = " " | "\t" | "\n" | "\r\n" ;
comment          = line_comment, ( "\n" | "\r\n" | EOF ) ;
line_comment     = "//", { ? any scalar except CR or LF ? } ;
```

The `EOF` alternative in `comment` is lexical only; it does not let `line_end` omit its newline. A pattern must therefore start with exactly one `language go` directive on its own logical line. Keywords are lowercase and case-sensitive. A required separator prevents a keyword from being read as a prefix of another token. Comments have no meaning inside snippets or regex literals. Bare CR, invalid UTF-8, empty `and`/`or` blocks, one-element `and`/`or` blocks, and omitted commas are syntax errors.

There are no infix query operators, parentheses, implicit sequences, or precedence rules. Unary prefix operators associate right-to-left, so `not maybe` followed by a snippet means `not (maybe snippet)`. The optional postfix `where` applies to the complete `prefix` immediately before it; to constrain a unary operand instead, place that operand in an `and` or `or` block. A second `where` is invalid. Blocks are comma-separated, evaluated left to right, and may have one trailing comma. These rules make every parse unique.

Valid examples are:

```grit
language go
`exec.Command($args)`
```

```grit
language go
and {
  `exec.Command($args)`,
  not `exec.CommandContext($args)`,
} where {
  $args <: r"^ctx,",
}
```

```grit
language go
contains `http.Client{$_}`
```

A regex is a predicate, not a top-level structural search: it is syntactically accepted as a `query` so it can occur after `<:`, but a top-level regex has no candidate text and produces `PATTERN_INVALID_CONTEXT`.

### 1.1 Identifiers and metavariables

Metavariable names are ASCII and case-sensitive. `$x` and `$X` differ. `$_` is the anonymous wildcard: every occurrence is independent, it is never recorded, may not be used on the left of `<:`, and does not participate in equality. Names beginning with a digit, non-ASCII names, bare `$`, and names beginning with `_` are rejected. Go identifiers written literally in snippets follow the supported Go parser's rules, independently of metavariable names.

Inside a snippet, `$name` is recognized before Go parsing. A backtick snippet is decoded by replacing `\`` with a backtick and `\\` with a backslash; every other backslash escape is invalid. This allows Go raw-string tokens to be represented without terminating the snippet. Regex escapes are decoded as listed by the grammar and the result is compiled with Go's RE2 syntax. Regex matching is unanchored unless the author writes anchors. It is applied to the exact UTF-8 source bytes covered by a binding, converted to a string; a list binding uses the single span from its first element's start through its last element's end, including intervening source. An empty list supplies the empty string.

## 2. Go snippet parsing

After metavariables are replaced by typed placeholders, a snippet is tried in this fixed context order:

1. one Go expression;
2. one Go type;
3. one Go statement;
4. a non-empty Go statement list;
5. one Go declaration;
6. a non-empty Go declaration list;
7. a complete Go file, including its `package` clause.

A context succeeds only when all decoded snippet input is consumed. Role inference is structural, not a scoring search: one skeleton parse represents every occurrence by a source-ordered unique generic identifier. Ordinary roles come from that identifier's actual Go ancestor/field, while whole import specs, import alias/path pairs, grouped declaration specs, type specs, and top-level declarations are resolved only inside their grammar-local component. Import components have at most the grammar's one alias and one path. The lexical component scan ignores literals and same-line comment punctuation, but records newlines inside block comments because Go treats them as semicolon-insertion separators. The pinned Go grammar is deterministic, so current inference produces exactly one assignment and performs one authoritative whole parse after the skeleton parse. Recovery nodes are evidence for locating a component, not alternate interpretations. There is no global role product, greedy repair, or Hamming-occurrence search. Grammar-component alternatives are represented component-wide (including both members of an import alias/path pair) so the validation path can accept multiple complete assignments defensively if a future grammar or edge case derives them. At most one assignment is emitted per component, so a context performs at most `metavariable occurrence count + 2` whole parses including the skeleton parse; current inference performs two. The number of ordinary placeholders is bounded only by the pattern byte limit.

The first successful context wins. The current deterministic Go grammar yields one inferred interpretation at that priority. The implementation nevertheless restores and canonicalizes every grammar-derived assignment supplied by inference: duplicate restored templates count once, and more than one distinct template produces the defensive `PATTERN_AMBIGUOUS_SNIPPET` diagnostic. Error-recovery readings are not candidates. Lower contexts are never considered after a valid candidate. A bare identifier is therefore an expression, and an expression statement uses the expression interpretation. Package fragments, unbalanced fragments, metavariables embedded in literals/comments or adjacent without grammar punctuation, and placeholders used where the selected Go grammar category cannot occur are rejected.

Synthetic parse scaffolding is not part of the resulting template. Authored statement and declaration lists use explicit `statement_sequence` and `declaration_sequence` template roots (including a one-item sequence when an explicit semicolon must be retained); only an authored complete file uses `source_file`. Every decoded metavariable occurrence must restore to exactly one slot before a candidate is accepted. A metavariable occupying an entire repeated grammar field is `SlotMany`, even when only one value is written. This includes direct name fields under `var_spec`, `const_spec`, `parameter_declaration`, and `field_declaration`, direct type fields under `type_case`, and a whole `type_elem` under an interface (so it is ready to bind either method-like or type-union interface elements), in addition to explicit list-node children. Type/name fields in those same ancestors remain singular when the grammar field itself is not repeated.

The supported source grammar is the Go language version declared by the implementation for `gritql-go-v1`; that exact version must be reported in evaluation metadata and must not vary during one evaluation.

## 3. Structural matching and bindings

A source file is parsed into a lossless Go concrete syntax tree. A structural value is normalized recursively as:

- the node kind;
- every ordered named child and unnamed punctuation/operator/token child;
- for a leaf token, its token kind and exact UTF-8 lexeme.

Whitespace, line terminators, comments, and automatically inserted semicolons are trivia and are omitted. Explicit semicolons, parentheses, delimiters, operators, identifier spelling, literal spelling, and escape spelling remain. No formatting, Unicode, numeric, or string-literal canonicalization occurs. Two node bindings are structurally equal exactly when these normalized trees are equal. Two list bindings are equal exactly when their lengths match and corresponding elements are structurally equal. This definition is used both for repeated names and branch merges. A node range starts at its first non-trivia token and ends after its last non-trivia token, so leading and trailing trivia never enters a match range; trivia between those tokens remains in the returned source slice.

A named metavariable's first occurrence binds the subtree permitted by its snippet grammar position. Later occurrences of that name must be structurally equal. A metavariable in an entire repeated-child slot may bind a consecutive list of zero or more children; it cannot splice through punctuation or across different parents. Repeated-list equality compares only corresponding normalized elements; separators are never equality members, so a trailing comma does not distinguish `f(a,)` from `f(a)`. Separators authored as literal template/node syntax, including explicit semicolons, remain structurally significant. List choice is greedy: try the longest sequence first, then shorter sequences, while alternatives and later snippet elements are tried left to right. The first complete structural match wins for that candidate. This deterministic backtracking rule also applies to multiple list metavariables. Bindings retain source order and are scoped to one candidate evaluation. Every list binding has one overall span for its grammar slot plus one ordered, non-overlapping element range per structural element. A non-empty overall span starts at the first normalized element and ends after the last normalized element, including intervening trivia and separators but excluding leading and trailing separators. An empty list has no element ranges and has a zero-width overall span immediately before the next concrete token, or immediately after the previous concrete token when there is no next token.

A `where` block is evaluated after its prefix succeeds. Constraints are evaluated left to right. Its left side must already have a binding from the prefix or a committed earlier constraint; this v1 grammar has no construct that can create such a binding solely on a left side. `$x <: P` runs `P` with `$x` as its candidate (or each list element in source order, succeeding once for each successful element). For `r"..."`, it instead tests the binding text as specified above. Every constraint must succeed.

## 4. Evaluation model

The evaluator visits named source AST nodes in preorder: parent before children, children in grammar order, with equal-offset children in grammar order. Unnamed tokens are structural children but are not traversal candidates. At every repeated-child position in the pinned Go grammar (identified by parent kind plus field name, including an empty field name), it also visits each non-empty consecutive element list exactly once, longest first and then by increasing first-element index. This includes statements, declarations, arguments, parameters, fields, repeated names, and `type_elem` members. A list candidate's range runs from its first normalized element through its last and excludes leading or trailing separators. Empty lists are bindable while matching a surrounding snippet but are not traversal candidates.

Queries return zero or more match records. Each record contains a primary range and a binding map:

- A snippet matches the current candidate structurally and returns that candidate's range.
- A regex is valid only as a `<:` predicate and returns its input binding's range.
- `and { A, B, ... }` evaluates every member against the same candidate. It forms the left-to-right Cartesian join of successful records, retaining only compatible bindings. Its range is the smallest byte span covering all member ranges.
- `or { A, B, ... }` evaluates every branch against the same candidate in source order and concatenates their records in branch order. It does not stop at the first success.
- `not P` succeeds once, with the current candidate's range and no new bindings, exactly when `P` returns no record.
- `maybe P` returns `P`'s records when any exist. If none exist, it succeeds once with the current candidate's range and no new bindings. Absence is not represented by a null or empty binding.
- `contains P` evaluates `P` at the current candidate and every named descendant in preorder. Each success retains `P`'s range and bindings.
- `within P` evaluates `P` at the current candidate and then each named ancestor, nearest first. Each success keeps the original current candidate's range, while retaining compatible bindings from `P`.
- A postfix `where` filters its prefix records and preserves each prefix range.

Containment is therefore reflexive: a node contains and is within itself. A *strict descendant* or *strict ancestor* excludes that node; v1 has no spelling that requests strict containment. For list candidates, containment uses the lowest common parent: self means that exact list, descendants are named nodes inside its elements, and ancestors begin at the common parent.

Each prefix, block branch, constraint, `not`, and `maybe` is a binding transaction. Failure rolls back all writes. Successful `and` members commit compatible unions; conflicting repeated bindings reject only that joined record. Each `or` branch starts from the same incoming map and commits only into its own records. `not` always discards child bindings. A successful `maybe` commits its child bindings, while its absent case commits none. No binding leaks between top-level candidates, findings, or files.

A top-level query runs once at every traversal candidate. `not` and absent `maybe` can consequently produce findings when used at top level; authors should normally use them inside `and` or `where`. Every returned top-level record becomes a finding using the composition range above.

## 5. Source, paths, ranges, ordering, and deduplication

Source must be valid UTF-8. Offsets are zero-based UTF-8 **byte** offsets into the unmodified file; ranges are half-open `[start_byte,end_byte)`. Lines and columns are one-based; a column is one plus the number of Unicode scalar values since the line start, not a byte count or display-cell width. LF advances one line. In CRLF, the pair advances one line and neither scalar belongs to the next line. Offset/range conversion must use the original bytes. Returned matched text is exactly that byte slice.

Input paths are converted to repository-relative slash-separated paths by replacing the host platform's separator with `/`, removing `.` segments, and resolving `..` only when it remains inside the repository root. Slash input is canonical on every host. Backslash is a separator and drive prefixes are rejected on Windows only; on POSIX, backslash and colon are ordinary filename characters. Absolute paths, paths escaping the root, NUL, and empty normalized paths are rejected. No case folding or Unicode normalization is performed. Path and string collation is lexicographic order of unsigned UTF-8 bytes.

A finding has `path`, `start_byte`, `end_byte`, `start_line`, `start_column`, `end_line`, `end_column`, `pattern_id`, `message`, and `bindings`. `bindings` is serialized by metavariable name in UTF-8 byte order. A node binding contains `kind`, one `range`, and one normalized structural node. A list binding contains `kind`, one overall `range`, ordered `ranges` for its elements, and an equally sized array of normalized structural nodes.

Before output, findings are sorted by this complete key:

1. normalized path;
2. `start_byte`;
3. `end_byte`;
4. `pattern_id`;
5. message;
6. canonical binding serialization (UTF-8 JSON with sorted object keys, no insignificant whitespace);
7. canonical complete finding serialization as the final total-order tie-breaker.

Two findings are duplicates only when their canonical complete finding serializations are byte-identical. Adjacent duplicates are removed after sorting. Line/column values never participate independently because byte offsets determine them.

### 5.1 CLI local-plus-remote composition

`grepple grit` and explicit `--local` evaluate only the current directory. Explicit `--remote` or `--server` evaluates the local directory and queries the authenticated router, then applies one client-side normalization and paging pass. When the checkout's Git remote can be canonicalized as `owner/repository`, that identity is sent in `exclude_repositories`; findings attributed to it are also discarded client-side in case a backend ignores the selector. Findings from unrelated repositories are retained even when their paths match local paths.

The composed ordering prepends repository identity to the v1 finding key, normalizes path separators, and then uses range, pattern identity, stable binding content, and the complete finding as deterministic tie-breakers. Only byte-identical complete findings are removed. User `skip` and `limit` are applied once to this global sequence. The client fetches bounded remote pages only until it has enough candidates for that window, and each request respects the `MaxGritPageLimit` server cap.

Statistics are added with integer saturation. Metadata must agree, while diagnostics, truncations, and shard errors from successful responses are retained. A successful response with shard errors is therefore a renderable partial result; transport, authentication, cancellation-before-transport, and incompatible-metadata failures remain command errors.

## 6. Errors and diagnostics

Pattern lexing/parsing, unsupported syntax, invalid context, and invalid regex reject the entire pattern and produce no findings from it. A source parse error rejects that entire file, emits exactly one `SOURCE_PARSE` diagnostic for the parser's earliest error (lowest byte offset, then parser message byte order), and produces no partial finding for that file. Other files may still be evaluated.

Diagnostics are separate from findings and contain these required fields: `code`, `class`, `severity` (`error`), `message`, `pattern_id` (string or null), `path` (normalized string or null), and `range` (the same byte/line/column fields as a finding, or null). Messages may improve, but code and class are stable API. Diagnostics are sorted by path (null first), range start (null first), code, pattern ID (null first), and message, all string comparisons using unsigned UTF-8 bytes.

Every `PATTERN_*` diagnostic and `LIMIT_PATTERN_BYTES` identifies its submitted pattern with a non-null `pattern_id`, has a null `path`, and has a non-null range into that pattern's query source. The deterministic v1 convention is to report the whole submitted pattern, including the `language go` header: `[0,len(pattern UTF-8 bytes))`, with scalar-based line and column coordinates computed exactly as for source files. This convention also applies when the error is localized or the byte limit is crossed earlier. Resource and runtime diagnostics other than `LIMIT_PATTERN_BYTES` may have a null range.

Stable v1 codes are:

| Code | Class | Meaning |
| --- | --- | --- |
| `PATTERN_PARSE` | `pattern` | malformed token stream or EBNF violation |
| `PATTERN_UNSUPPORTED` | `unsupported` | recognized out-of-contract construct |
| `PATTERN_INVALID_CONTEXT` | `pattern` | valid token used in a forbidden context |
| `PATTERN_INVALID_SNIPPET` | `pattern` | snippet is not valid Go in any supported context |
| `PATTERN_AMBIGUOUS_SNIPPET` | `pattern` | multiple distinct grammar-derived assignments at the selected snippet priority (defensive) |
| `PATTERN_INVALID_REGEX` | `pattern` | invalid RE2 expression or regex limit exceeded |
| `SOURCE_INVALID_UTF8` | `source` | source is not valid UTF-8 |
| `SOURCE_PARSE` | `source` | Go source parse failure |
| `PATH_INVALID` | `source` | path cannot be normalized safely |
| `LIMIT_PATTERN_BYTES` | `resource` | pattern byte limit exceeded |
| `LIMIT_SOURCE_BYTES` | `resource` | source byte limit exceeded |
| `LIMIT_PARSE_DEPTH` | `resource` | parse/tree depth limit exceeded |
| `LIMIT_CANDIDATES` | `resource` | traversal candidate limit exceeded |
| `LIMIT_AST_STEPS` | `resource` | AST/match step budget exceeded |
| `LIMIT_FINDINGS` | `resource` | rule finding limit reached |
| `LIMIT_TIME_FILE` | `resource` | per-file deadline exceeded |
| `LIMIT_TIME_BATCH` | `resource` | batch deadline exceeded |
| `LIMIT_MEMORY` | `resource` | accounted memory limit exceeded |
| `EVALUATION_CANCELLED` | `cancelled` | caller cancellation observed |
| `INTERNAL_ERROR` | `internal` | evaluator invariant or unexpected failure |

Unsupported constructs should use `PATTERN_UNSUPPORTED`, not `PATTERN_PARSE`, when the lexer can identify them. Internal errors must not expose secrets or stack traces in `message`.

## 7. Resource bounds and partial results

Defaults are mandatory. A host may lower them, but may not exceed the hard maxima while claiming v1 conformance.

| Resource | Default | Hard maximum |
| --- | ---: | ---: |
| decoded pattern body | 256 KiB | 1 MiB |
| one source file | 10 MiB | 50 MiB |
| pattern/Go tree depth | 256 | 1,024 |
| traversal candidates per file | 250,000 | 1,000,000 |
| AST/match steps per file | 10,000,000 | 100,000,000 |
| findings per rule per batch | 10,000 | 100,000 |
| per-file wall time, including source acquisition | 2 s | 10 s |
| batch wall time, including source acquisition | 30 s | 300 s |
| eligible files per filesystem scan | 100,000 | 1,000,000 |
| acquired source bytes per filesystem scan | 1 GiB | 16 GiB |
| filesystem scanner workers | 4 | 64 |
| accounted live memory per evaluation | 128 MiB | 512 MiB |
| decoded regex length | 16 KiB | 64 KiB |
| RE2 compiled program instructions | 100,000 | 500,000 |

The evaluator uses Go RE2 semantics; matching is linear-time and no regex timeout or backtracking engine is permitted. Regexes that exceed the byte or compiled-program bound fail during pattern validation with `PATTERN_INVALID_REGEX`.

Files are evaluated sequentially in normalized path order for contractual output, even if an implementation computes speculatively. A file is transactional: source parse failure, candidate/depth/memory/file-time limit, cancellation observed during that file, or internal error discards every finding from that file. Previously committed files remain except for caller cancellation or a batch timeout, which returns **no findings at all**. This all-or-nothing cancellation rule makes its result independent of scheduling and checkpoint timing.

The finding cap is applied separately to each `pattern_id` in the globally sorted, deduplicated stream: retain the first N findings for each rule and emit one pathless `LIMIT_FINDINGS` diagnostic for every truncated rule. Retained findings and all diagnostics are globally resorted and deduplicated before publication. Oversized/invalid files produce their diagnostic and no findings, then evaluation continues. A batch memory failure discards the current file and stops after previously committed files. Other batch resource failures stop before the next normalized path. Caller cancellation or batch timeout discards all findings, retains already-determined non-cancellation diagnostics, and adds exactly one sorted, deduplicated cancellation/timeout diagnostic. No speculative or half-evaluated record is returned.
Implementations must check cancellation and deadlines before every file, at least every 1,024 traversal candidates, and before committing a file. `LIMIT_CANDIDATES` counts only the named-node and consecutive-list traversal candidates described above; it does not count AST inspection, query operations, constraints, or matcher backtracking. Those operations consume the separate AST/match step budget and report `LIMIT_AST_STEPS`. Accounted memory includes parsed pattern/source trees, candidate state, bindings, findings, and regex programs; exceeding it must be detected at allocation-accounting boundaries rather than relying on process OOM behavior.

The filesystem scanner accepts repository-scoped candidate paths rather than discovering files itself. It normalizes and canonical-deduplicates those paths, retains only Go candidates, applies include globs followed by exclude globs, and evaluates the survivors in normalized path order. An absent language is inferred as Go only for a `.go` path; an explicit non-Go language, an unmatched glob, and a file containing a NUL byte are skipped without diagnostics. Files are acquired with a bounded read of at most the lower source/memory allowance plus one detection byte; `os.ReadFile`-style unbounded acquisition is forbidden. Worker count is bounded and may be reduced below the requested/default count by the effective memory allowance. Every accepted non-binary file is parsed once, and its tree is closed after evaluation.

`max_files` and `max_total_bytes` scanner truncations are reported separately from language/glob/binary skips and from evaluation diagnostics. Their stable records contain `reason`, effective `limit`, and the count of normalized eligible paths omitted. File-count truncation retains the first normalized paths. Aggregate-byte truncation commits only the longest normalized prefix whose acquired bytes fit. Worker completion order does not affect findings, diagnostics, statistics, or truncation records. Scanner cancellation and batch timeout follow the same all-or-nothing finding rule as batch normalization and may return before an in-progress filesystem read finishes because a generic `fs.FS` read is not interruptible.

Candidate optimization is conservative. Eligible anchors are exact significant snippet leaf tokens containing at least one Unicode letter, number, or underscore. `and` and mandatory structural `where` constraints combine their anchors; `or` retains only anchors shared by every branch; `contains` and `within` preserve their operand anchors; and `not`, `maybe`, and regex constraints contribute none. A query with no eligible mandatory literal has no safe anchor and requires the complete scoped file set. Callers may require any or all reported anchors, but must not infer additional literals. Disabling anchor optimization changes candidate volume only, never findings.

## 8. Explicitly unsupported syntax and behavior

The following are recognized but unsupported and fail closed with `PATTERN_UNSUPPORTED`:

- rewrites or replacement arrows such as `=>`;
- pattern definitions, named definitions, and calls to them;
- imports, modules, libraries, and remote patterns;
- multifile patterns, sequential patterns, and cross-file state;
- named AST constructors;
- language prefixes or qualifiers on snippets/patterns (the only language syntax is the required header);
- `as` captures;
- assignments, predicates/functions, custom or foreign functions;
- any equality or inequality operator (including `==` and `!=`); equality exists only through repeated metavariable binding;
- any operator, literal, comment form, or delimiter absent from the EBNF.

Non-Go targets, type checking, name resolution, data flow, network access, shell execution, repository writes, interactive input, and source rewrites are behaviorally unsupported. `gritql-go-v1` is native and detection-only.

## 9. Security and performance gates

The kernel is a library-only detector. Production code in `gritql` and its parser facade performs no network access, process execution, dynamic extension loading, or source writes. It reads only caller-supplied bytes or explicitly scoped `fs.FS` candidates. Queries cannot invoke foreign functions, imports, shell commands, or remote registries. Paths and globs are validated before source acquisition. Regexes use bounded RE2 compilation, evaluation loops charge explicit candidate/step/time/memory budgets, scanner acquisition is bounded, and compiled-program caches contain immutable IR only—never source ASTs.

The following Linux/amd64 baseline was recorded on 2026-09-08 using an Intel i7-1165G7 and `-benchtime=100ms`:

| Benchmark | Time/op | Bytes/op | Allocations/op | Review threshold |
| --- | ---: | ---: | ---: | --- |
| compile representative query | 0.31 ms | 49 KB | 1,149 | investigate above 1 ms, 100 KB, or 2,000 allocs |
| evaluate representative file | 0.63 ms | 212 KB | 2,381 | investigate above 2 ms, 500 KB, or 5,000 allocs |
| shared parse, 100 files / 2 rules | 69 ms | 23.6 MB | 260,936 | investigate above 200 ms, 50 MB, or 500,000 allocs |
| anchored scan, 100 files | 41 ms | 12.6 MB | 135,887 | investigate above 120 ms, 30 MB, or 300,000 allocs |
| unanchored scan, 100 files | 51 ms | 16.1 MB | 170,855 | investigate above 150 ms, 40 MB, or 400,000 allocs |
| pre-cancelled 100-file scan | 17 µs | 21 KB | 143 | investigate above 200 µs, 100 KB, or 500 allocs |

Thresholds are review gates rather than portable pass/fail assertions: CPU, Go version, tree-sitter build, and race instrumentation affect absolute numbers. The invariant suitable for CI is that the shared two-rule scan parses exactly 100 files, not 200. Run the reproducible gates with:

```bash
go test ./gritql -run '^$' -bench=. -benchmem
for target in CompileNoPanicDeterministic CompileSnippetNoPanicDeterministic BindingRollbackNoPanicDeterministic EvaluateFileNoPanicDeterministic NestedTraversalNoPanicDeterministic ValidateGlobsNoPanicDeterministic; do
  go test ./gritql -run '^$' -fuzz="^Fuzz${target}$" -fuzztime=10s
done
```

## 10. Conformance and versioning

A conforming implementation must fixture-test every grammar production, snippet context, binding transaction, range rule, diagnostic code, ordering key, limit outcome, and unsupported category. Evaluation metadata publishes contract `gritql-go-v1`, canonical Go grammar identifier `go1.25`, and every effective limit. The parser implementation pin (`tree-sitter-go@0.25.0`) is reported separately and is not substituted for the canonical grammar identifier.

The accepted language is closed: adding syntax or changing matching, range, ordering, cancellation, or diagnostic classification requires a new compatibility contract. Clarifications that do not change observable behavior may retain the `gritql-go-v1` name. Existing stable codes may not be reassigned.
