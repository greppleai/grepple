# Directory architecture

`grepple architecture` provides a language-neutral physical view without treating directories as package, module, or layer intent.

## Directory orientation

```bash
grepple architecture directory --depth 2 --max-nodes 80 --compact .
grepple architecture directory --json src services
```

`grepple-directory-architecture-v3` reports hierarchical directories, recursive file/language/declaration counts, repository import roots, production/test/fixture/generated/vendor classifications, callable visibility, adapter-evidenced entrypoints and routes, exact declaration ranges, and cross-directory relations. Entrypoint and route claims are emitted only when an owning language adapter supplies a reliable contract; Go currently recognizes `package main`'s `main` function and package-qualified `net/http.Handle`/`HandleFunc` registrations, including Go 1.22 method-pattern strings. Relations are independently labeled `resolved-call`, `import`, or `type-reference`; each relation carries source-class totals and exact evidence. Coverage reports resolved, ambiguous, unresolved, unqualified, and adapter-unsupported import/type analysis. Each selected code file is parsed once and the same caller-owned `parser.Document` supplies its outline and cached navigation facts. `--max-files` limits source analysis; compact `--depth`, `--max-nodes`, and `--max-output-bytes` affect presentation. Sources and truncation are always explicit.

Repository `grepple.json` ignores apply before analysis. Explicitly named files bypass ignores and emit a notice. Use `--production-only` to remove conventionally classified tests, fixtures, generated files, and vendor files from recursive analysis. Before relying on absence or completeness, run `grepple sources explain --compact PATH` to inspect the loaded config digest and exclusion counts. Large JSON follows the shared spill policy and may produce a `grepple-artifact-v1` descriptor instead of injecting the complete document into stdout.

For Go, repository import roots come from the nearest module identity for every selected file plus unambiguous local `go.mod`/`go.work` replacements. Nested modules retain their nearest identity; conflicting replacements remain unresolved. This context is rebuilt after path-neutral cache loading, so focused/full and cold/warm runs do not inherit stale repository identity.

## Declaration resolution

```bash
grepple architecture resolve --symbol Document --compact .
```

`grepple-architecture-resolve-v1` searches parser outlines, so it covers types and nested declarations as well as callables. Results are deterministic path/source order and retain every same-name match. Visibility is `unknown` when no adapter-owned navigation fact establishes it.

## Relation evidence

```bash
grepple architecture why rulespec search --compact rulespec search
```

`grepple-architecture-why-v2` reports all available relation kinds between the selected directories. Resolved calls remain restricted to `exact`, `import-resolved`, or `context-resolved` confidence. Adapter-owned Go and JavaScript/TypeScript import facts add import-only edges; imported callable parameter/result/local/field types add type-reference edges when the local target directory resolves uniquely. Every item includes relation kind, source classification, source path, line, and confidence. Ambiguous and unresolved facts remain coverage counts rather than asserted edges.

Absence is not proof that no dependency exists: coverage explicitly lists languages without import facts and counts unresolved evidence. Build-system, generated, reflective, registration, data-flow, and runtime relationships are not represented. Use navigation graph queries for callable impact and boundary analysis for heuristic architecture review.
