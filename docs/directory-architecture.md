# Directory architecture

`grepple architecture` provides a language-neutral physical view without treating directories as package, module, or layer intent.

## Directory orientation

```bash
grepple architecture directory --depth 2 --max-nodes 80 .
grepple architecture directory --relations --depth 0 --max-nodes 0 --max-output-bytes 0 .
grepple architecture directory --mermaid --depth 0 --max-nodes 0 --output architecture.mmd .
grepple architecture directory --json src services
grepple architecture directory --server http://127.0.0.1:8080 --repo OWNER/REPO@tag~v1.2.3
```

The default human output groups relation kinds and evidence under each directory pair and lists `unconnected PATH` for visible leaf directories without visible cross-directory relations; it does not repeat directory/declaration inventories. The complete `--json` report retains these inventories and all relation evidence. `grepple-directory-architecture-v5` reports the selected source-file inventory, hierarchical directories, recursive file/language/declaration counts, repository import roots, production/test/fixture/generated/vendor classifications, callable visibility, adapter-evidenced process entrypoints, exact declaration ranges, and cross-directory relations. Go recognizes `package main`'s `main` function; Java, Kotlin, and C# recognize conservative language-defined main signatures; C/C++ recognize syntax-evidenced global `int main`; and Rust recognizes a top-level `main` only in selected `src/main.rs` and `src/bin` binary crate roots. Relations are independently labeled `resolved-call`, `import`, or `type-reference`; each relation carries source-class totals and exact evidence. `--relations` groups those kinds by directory pair and emits only `R FROM -> TO kinds=...` lines for high-level dependency visualization. `--mermaid` renders that same grouped projection as a deterministic left-to-right flowchart and writes it to stdout or `--output PATH`. Every visible directory receives a deterministic high-saturation color: its node border acts as the legend and every outgoing link uses the same stroke color. Colors are generated at a shared relative luminance chosen for contrast against both light and dark Mermaid themes, while golden-angle hue spacing keeps adjacent directory colors distinct. Coverage reports resolved, ambiguous, unresolved, unqualified, and adapter-unsupported import/type analysis. Each selected code file is parsed once and the same caller-owned `parser.Document` supplies its outline and cached navigation facts. `--max-files` limits source analysis; `--depth` and `--max-nodes` affect text and Mermaid presentation, while `--max-output-bytes` bounds text only. Sources and coverage remain available in human and JSON output; Mermaid embeds node truncation as a comment. Use unbounded limits when a complete relations projection is required.

Repository `grepple.json` ignores apply before analysis. Explicitly named files bypass ignores and emit a notice. Use `--production-only` to retain only files classified `production` by fresh `grepple.yaml` entries; absent, stale, or invalid classifications are `unknown`, not inferred from paths. Before relying on absence or completeness locally, run `grepple sources explain PATH` to inspect the loaded config digest and exclusion counts. Large JSON follows the shared spill policy and may produce a `grepple-artifact-v1` descriptor instead of injecting the complete document into stdout. Remote analysis requires one exact indexed `OWNER/REPO[@REF]` selector; JSON uses `grepple-remote-analysis-v1` to retain selector, commit, completeness, notices, shard errors, and the nested versioned result.

For Go, repository import roots come from the nearest module identity for every selected file plus unambiguous local `go.mod`/`go.work` replacements. Nested modules retain their nearest identity; conflicting replacements remain unresolved. This context is rebuilt after path-neutral cache loading, so focused/full and cold/warm runs do not inherit stale repository identity.

## Focused navigation

Use `grepple --outline PATH` or `grepple graph resolve --symbol NAME` to locate declarations, and `grepple graph callers|callees --at PATH:LINE` for static call relations. Directory JSON retains relation evidence and coverage; unresolved, generated, reflective, and runtime dependencies cannot be ruled out from missing static edges.
