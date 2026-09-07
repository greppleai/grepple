---
name: local-grep-and-search
description: Use for any local text/content search, file listing by name/glob, or match counting in the current working directory — instead of grep, rg/ripgrep, ag, or find-for-content. Do not fall back to those tools. For repos you have not cloned or any remote/cross-repo target, use remote-grep-and-search.
---

# grepple — local search

`grepple` is a structure-aware grep for agents. By default it searches the **current
working directory** (it stays local unless you opt into remote — see the
`remote-grep-and-search` skill). It works on **any text file** — code, Markdown,
config, prose, logs — searching line-by-line, and adds tree-sitter structural
context so a match is shown in context:

- For supported **code** languages the enclosing function/class is shown, unrelated
  lines collapsed (`// … N lines collapsed …`).
- For **Markdown** the enclosing heading chain (`# Doc › ## Section › ### Sub`)
  precedes the matched line.
- Other text files fall back to plain matching lines.

Results are in **deterministic path order** (`dir/file`) — there is no relevance
ranking, so you refine by narrowing (globs, `--repo`, a tighter pattern), not by
trusting a score. `.gitignore` entries and `.git` directories are excluded.

## Token-efficient narrowing (recommended workflow)

Prefer cheap, bounded probes before dumping file bodies — it keeps output (and token
use) small and lets you zero in fast:

1. **Locate** — `grepple PATTERN --count` returns a per-directory/repo tally
   (`name<TAB>N files<TAB>N matches`, hottest first) over the *complete* match set,
   with a tiny fixed-size payload. Use it to see *where* a term concentrates.
2. **List** — `grepple PATTERN --files-with-matches` lists matching file paths
   (`grep -l`) with no bodies.
3. **Read** — pull only what you need with a bounded search:
   `grepple PATTERN --limit 5`, or open the file at a known line.

Rules of thumb:
- Default `--limit` is **20**; broad content searches are capped (with a stderr hint).
  Pass `--limit 0` only when you truly need everything (remote pages are capped at
  100 regardless — see the remote skill).
- Prefer `--count` / `--files-with-matches` / `--line-only` over the default segment
  output when you don't need surrounding code — they skip tree-sitter parsing and
  return far fewer tokens.

## Search flags

| Flag | Meaning |
| --- | --- |
| `--regex` | Treat PATTERN as a regex (this is the **default**) |
| `-F`, `--fixed-strings` | Treat PATTERN as a literal string (substring match) |
| `-i`, `--ignore-case` | Case-insensitive matching |
| `-n`, `--line-number` | Include line numbers (on by default) |
| `--line-only` | Print only the matching lines |
| `-C N`, `--context N` | Print N lines of context around matches |
| `-l`, `--files` | List files whose **path** matches the glob (filename search, **not** contents) |
| `--files-with-matches` | List paths of files whose **contents** match, like `grep -l` |
| `-c`, `--count` | Per-directory/repo match tally (`name\tfiles\tmatches` + total), compact — best first probe. Composes with a glob, e.g. `grepple -F "react" "**/package.json" --count` |
| `--json` | Full JSON results; `--json-matches` for compact JSON |
| `--max-files N`, `--max-segments N` | Limit matching files / result segments per file |
| `--skip N` | Skip the first N result files (deterministic path order; default `0`) |
| `--limit N` | Return at most N result files (**default `20`**; `--limit 0` = all local results; a remote server caps each page at **100**) |
| `-O`, `--outline` | Print each file's structural outline instead of searching (see below) |

> `grepple` is local-first: it only contacts a server if you pass `--remote`/`--server`
> (see the `remote-grep-and-search` skill). `--local` forces local-only (the default).

## Glob / path rules

- Globs use Go `filepath.Glob` syntax, extended with `**` to cross directories
  (e.g. `**/*.yaml`, `charts/**/values.yaml`).
- Omit globs to search recursively from the CWD; matching a directory searches it recursively.
- `-l/--files` is a **filename** search: the positional is a **glob** matched against the whole path, and `*` does **not** cross `/` — use `**/*ping*` (not `ping` or `*ping*`). To list files **containing** text, use `--files-with-matches` instead.
- With `-l`/`-O` you can combine a filtering glob with a **directory** to scope it: `grepple -l "**/*.tsx" testdata` lists `.tsx` files **under** `testdata` (a bare directory acts as a scope root; a glob acts as a filter). A directory on its own still lists everything beneath it.
- `--regex` and `--fixed-strings` are mutually exclusive.

## Example calls

```bash
grepple "console\\.log" "src/*.ts"    # regex (default) within a glob
grepple -F "TODO(" internal            # literal search under internal/
grepple -i -C 2 "timeout" "**/*.go"    # case-insensitive, 2 lines of context
grepple -l "**/*.yaml"                 # list files whose NAME matches (glob, not a pattern)
grepple ping --files-with-matches      # list files whose CONTENTS contain ping (grep -l)
grepple -c "func "                     # where do matches concentrate? (tally)
grepple "bar" --limit 20               # first 20 files in path order (cheap; see note)
grepple "bar" --skip 20 --limit 20     # next page of 20 files (path order)
grepple "client secret" README.md      # Markdown: match shown under its heading chain
```

## Paging & performance (`--skip` / `--limit`)

Results are returned in **deterministic path order** (`dir/file`) — no relevance
ranking — then paged. Defaults: `--skip 0`, **`--limit 20`** (broad queries are
capped so they don't dump every match; when the default cap is hit, grepple prints a
hint to stderr). Pass `--limit 0` for unbounded local results (a remote server caps
each page at 100 — page with `--skip N --limit 100`), or page: `--limit N` for the
first page, `--skip N --limit N` for subsequent pages.

Prefer `--limit` for broad queries. grepple only tree-sitter-parses the files it
actually returns, so a small `--limit` makes broad searches (e.g. very common
substrings like `bar`) far cheaper.

## Outline (structural map of a file)

`--outline` (`-O`) prints the structure of a file instead of searching its contents:
for code the definitions — classes, interfaces, functions, methods, structs/types,
fields — and for **JSON/YAML** the key/shape skeleton with values omitted, each with
its **line ranges**. Think of it as a generated header / `ctags` view. Great for
orienting in an unfamiliar or **large** file cheaply (it shows structure, not every
line) — especially big config/data files where you want the shape, not the payload.

```bash
grepple --outline internal/search/rules.go     # one local file
grepple --outline "internal/**/*.go"           # every file matching the glob
grepple --outline --json src/app.ts            # machine-readable {"files":[...]}
grepple --outline values.yaml                  # JSON/YAML: key tree, values omitted
grepple --outline --depth 2 large-config.json  # cap nesting (JSON/YAML only)
```

Output is `START-END<TAB>KIND<TAB>NAME`, nested members indented under their
container:

```
internal/router/rules.go	go
20-28	struct	ruleRegistry
112-130	method	(*ruleRegistry).upsert
216-262	method	(*ruleRegistry).fetchResults
```

- **Languages:** the tree-sitter set (Go, TypeScript/TSX, JavaScript/JSX, Java,
  Kotlin) get full symbol outlines; **Markdown** gets a heading outline (`h1`…`h6`,
  code-fence aware); **JSON/YAML** get a key/type tree (`object`/`array`/`string`/
  `number`/`bool`/`null`). Other file types produce an empty outline.
- **JSON/YAML specifics:** values are omitted (keys + types only); arrays show a
  `[N]` length and recurse only into object/array elements (scalar lists collapse to
  one line); scalars are typed via the resolved YAML tag. **Multi-document YAML**
  (`---`) is supported — each document is a `document [i]` node. `--depth N` caps
  nesting (0 = unlimited) for these formats.
- **Tiny files:** when the outline would be larger than the file itself (common for very small JSON/YAML), the plain-text `--outline` prints the raw file instead (same `path<TAB>language` header, then the contents) so you never spend more tokens than just reading it. `--json` always returns the structured symbols.
- Go methods appear at top level as `(*Type).Method` (faithful to source order).
- Composes with `--limit`/`--skip`; like search it defaults to the first `--limit 20`
  files (pass `--limit 0` for all). Cannot be combined with `--count` or
  `--files-with-matches`.

## Gotchas

- Piping `grepple ... | head` can exit 141 (SIGPIPE); the search still succeeded.
- **Piping INTO grepple works like grep/rg**: `cat build.log | grepple "ERROR"` or `go test ./... | grepple -F "FAIL"` — when stdin is piped and no path/glob argument is given, the stream is searched and reported as `<stdin>` (plain-text matching only). This is the blessed way to filter command output. Passing any path/glob selects the filesystem instead.
- Binary files and files with a NUL byte are skipped during content search but can
  still appear under `--files`.
- Supported tree-sitter languages get structural context/segments: `.ts/.tsx`,
  `.js/.jsx`, `.go`, `.java`, `.kt/.kts`. **Markdown** (`.md`) gets heading-section
  context (the enclosing `#`…`######` breadcrumb). Everything else uses plain-text
  line matching. (This shapes result *segments*, not ordering — there is no ranking.)
- To search repositories you have **not** cloned, fetch a remote file, browse a repo
  tree, or run org-wide saved searches, use the **`remote-grep-and-search`** skill.
