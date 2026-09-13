---
name: local-grep-and-search
description: Use for local content search, filename listing, match counting, structural orientation, exact declaration retrieval, and code navigation—instead of grep, rg/ripgrep, ag, or find-for-content. Do not fall back to those tools. Use remote-grep-and-search when the target is not in the current checkout.
---

# Local Grepple

Use Grepple to minimize retrieval turns and tokens, not merely as a grep replacement. Results are deterministic path order, not relevance-ranked; narrow deliberately instead of trusting the first hit.

## Choose the cheapest useful shape

- **Unknown scope:** start with `--count-summary` to learn the complete matched-file and matching-line breadth without retrieving bodies. Then use `--files-with-matches` or a scoped per-file `--count` to choose paths.
- **Need candidate paths:** use `--files-with-matches`; use `--files` only when matching filenames/globs.
- **Need file structure:** use `--outline` before reading a large or unfamiliar file.
- **Need implementation context:** default search returns enclosing structural segments and collapses unrelated code.
- **Need edit-ready context:** when a compatible provider has `enabled_by_default`, normal structural and `--line-only` searches already emit `HASH│LINE│content`; otherwise add `--anchors`. Edit directly without Read. Use `--no-anchors` when plain output is required. `HASH` mutates; 1-indexed `LINE` orients and composes with `--at`. Refresh both after edits.
- **Need only evidence lines:** use `--line-only` or bounded context; this avoids retrieving structural bodies. For parser-backed files, a match that begins a multi-line construct is located as `PATH:START-END:text`, so pass that range directly to Read or `--at`; ordinary lines remain `PATH:LINE:text`. When the match is inside a construct, add `--enclosing` to get `PATH:MATCH@START-END:text` and read the exact nearest syntax scope without another discovery call.
- **Know a navigation location:** use `--at PATH:LINE` (also accepts `PATH:START-END`) to retrieve the exact callable declaration instead of reading the file broadly.
- **Need the next code hop:** add `--related` to expose bounded callees and potential callers. This often avoids a second symbol search.
- **Need a short call chain:** use `--follow-related 1` first. Increase to 2–3 only when the extra inline context is worth the tokens.
- **Assessing a refactor, package split, ownership boundary, or impact:** do not stop at outlines and occurrence searches. Run `--related` for an immediate preview, then use `grepple graph callers --at PATH:LINE --depth N --compact SCOPE` when a deterministic multi-hop incoming subgraph can replace repeated caller searches. Use `graph callees` for the outgoing direction. Add repeatable `--language` or `--confidence` filters only when a mixed or lower-confidence graph creates noise; filtering candidates trades completeness for precision.

## Why navigation saves tool-call cycles

A normal text-search investigation often becomes a loop: locate an identifier, read its declaration, search each newly discovered call, then read those declarations. `--related` combines the first several hops into one bounded result: the enclosing declaration plus likely callers and callees. `--follow-related` can inline the next declarations as well. This reduces round trips and preserves the dependency neighborhood in one model-visible response.

Navigation also improves architectural understanding. Occurrence searches show where names appear; callers and callees show how responsibilities connect. For package extraction, command decomposition, shared-helper ownership, or change-impact analysis, those edges expose hidden coupling and challenge boundaries that look clean from filenames or outlines alone.

Use navigation deliberately rather than everywhere:

- Start with an outline when you only need inventory.
- Use a literal search when you only need occurrences.
- Use `--related` whenever the question involves control flow, coupling, ownership, impact, or the next code hop.
- For an architectural recommendation, inspect both feature entry points and shared-looking helpers before concluding that a boundary is viable.
- Prefer one scoped `--follow-related 1` call over a sequence of search → read → search calls when its bounded expansion answers the same question.

## Why navigation is opt-in

`--related` and `--follow-related` parse the selected source set to build a project-local declaration index. They are valuable for impact analysis and unfamiliar control flow, but wasteful for simple text checks.

Navigation is syntax-based, not type-checked:

- `exact` means a qualified callable identity matched directly; `import-resolved` means an explicit Go or TypeScript import identified the target package/module; `context-resolved` means declaration kind, file locality, or a direct receiver type safely narrowed candidates; `unique-terminal` means only one same-language terminal-name declaration was found.
- `[candidate; try --at PATH:LINE]` means ambiguity remains; use the suggested declaration location and verify rather than treating it as an exact call graph.
- `→` is a callee and `←` is a potential caller.
- Selected paths/globs define the navigation universe; include the relevant directory for cross-file edges.
- Callees and callers are bounded previews. If Grepple reports omitted-edge counts, narrow the scope or inspect the complete `grepple graph --json PATH` projection before making an impact claim.
- TypeScript and TSX share a namespace. Other languages are isolated. Go and TypeScript propagate direct types, source-ordered lexical construction, local and imported return signatures, and same-file typed member chains; cross-file field inference may remain a candidate.
- Expansion is bounded (two resolved callees per level, depth ≤3, cycle protection, shared line budget).

Navigation works locally in default structural output or full `--json` for Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, C#, C, C++, Rust, and Shell. It does not benefit Markdown, config, logs, or plain text.

## Narrowing rules

- Default `--limit` is 20. Keep it bounded; use `--limit 0` only when completeness is necessary.
- Human-readable output is capped at 16384 bytes by default. If the truncation marker appears, narrow the path/glob or use `--limit`, `--line-only`, `-l`, or `--count`; use `--max-output-bytes 0` only when unbounded output is genuinely required.
- Prefer `-F` for literal identifiers/snippets; JavaScript regex is the default.
- Grepple is not a complete grep flag clone. `-E` explicitly selects the default JavaScript-regex mode, and `-r` is accepted as a no-op because directory search is already recursive. Translate other grep flags to Grepple's output modes.
- Scope with a file, directory, or glob. `**` crosses directories.
- `--files` matches paths; `--files-with-matches` matches contents.
- `.git` and ignored files are excluded.
- Pipe command output into Grepple instead of grep: `go test ./... | grepple -F FAIL`.

## Minimal patterns

```bash
grepple -F 'Symbol' --count-summary         # complete breadth, independent of paging
grepple -F 'Symbol' --count src             # per-file counts for a narrowed scope
grepple -F 'Symbol' --files-with-matches    # identify candidate files
grepple --outline path/to/file.go           # orient cheaply
grepple -F 'Symbol' src --limit 5           # retrieve bounded structure
grepple --related -F 'Symbol(' src          # choose caller/callee next hops
grepple --line-only -F 'Symbol' src/file.go # locate lines; construct starts include PATH:START-END
grepple --line-only --enclosing -F 'call()' src/file.go # body match plus nearest syntax range
grepple --anchors -F 'Symbol' src/file.go    # explicitly skip Read with an anchor provider
grepple --no-anchors -F 'Symbol' src/file.go # override settings when plain output is required
grepple --at src/file.go:40-58               # retrieve a listed declaration
grepple --follow-related 1 -F 'Symbol(' src # inline one deliberate hop
grepple graph callers --at src/file.go:40 --depth 2 --compact src # bounded incoming graph
```

For repositories or files not present in this checkout, use `remote-grep-and-search`; do not clone merely to inspect them.
