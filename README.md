# Grepple

Grepple helps you find text, inspect code, and follow source relationships in a repository. It runs locally without a server. Searches return results in deterministic path order; narrow the files or query when you need a smaller answer.

## Install

Download a `grepple` binary for Linux, macOS, or Windows from [GitHub Releases](https://github.com/greppleai/grepple/releases). To build from source, you need Go 1.25 or later and a C compiler for the bundled Tree-sitter parsers:

```bash
git clone https://github.com/greppleai/grepple.git
cd grepple
make build
./bin/grepple --version
```

Use `make install` to install the locally built CLI. Run `grepple --help` for the current command and flag list.

## Explore a repository

Run these commands from the repository you want to inspect (replace example paths and symbols with your own):

```bash
grepple tree --depth 1                  # See the top-level files and directories
grepple tree src/                       # Expand one directory
grepple -F 'handleRequest' src/          # Search for literal text
grepple --outline src/server.go         # List declarations in a file
grepple --at src/server.go:42           # Read a declaration at a known line
```

Search works on plain text as well as source code. Supported languages get syntax-aware results; other files remain searchable as text. Use `grepple examples` for short, copyable workflows, or browse the [examples](examples/README.md).

## Find and narrow results

```bash
grepple 'handle(Request|Response)' src/  # Regular expressions are the default
grepple -F 'TODO' --files-with-matches .  # Only matching file paths
grepple --files '**/*.go'                # Files selected by path, not contents
grepple -F 'TODO' --count-summary .       # Complete match totals
grepple -F 'handleRequest' --line-only . # Matching lines with edit anchors
grepple -F 'handleRequest' --json .      # Structured output
```

A directory search is recursive. File globs can include `**`. Local results include source locations; eligible source lines carry `HASH│LINE│content` anchors for [safe, transactional edits](docs/write.md). Large human-readable results are bounded, while complete JSON is available for automation. See [output contracts](docs/output-contracts.md) for paging, limits, completeness, and JSON details.

## Follow code relationships

```bash
grepple graph resolve --symbol handleRequest .
grepple graph callers --at src/server.go:42 --depth 2 .
grepple graph callees --symbol handleRequest .
grepple architecture directory .
```

Graph results use source evidence and may report multiple candidates rather than guessing a runtime target. `architecture directory` gives you a source-linked map of the repository; see the [directory architecture guide](docs/directory-architecture.md). For an explicit syntax pattern, run `grepple grit --query-file checks.grit '**/*.go'`; the supported query language is described in [GritQL compatibility](docs/gritql-compatibility.md).

## Optional services

A compatible indexed server enables remote queries with `--server URL --repo OWNER/REPO`; local searching needs no server. See [versioned indexing](docs/versioned-indexing.md) for exact repository selectors. `grepple ask` offers optional AI-assisted, read-only repository research once a provider is configured; see [Ask research](docs/ask.md).

For repeated local directory or focused graph requests, start `greppled` in another terminal and add `--daemon` to the command. If the daemon is unavailable, Grepple performs the work locally.

## More guides

- [Supported file types and language features](docs/file-type-support.md)
- [Source selection and ignore rules](docs/source-scope.md)
- [Release history](docs/CHANGELOG.md)
- [Full documentation](docs/) and [worked examples](examples/README.md)

The previous long-form README is [archived verbatim](docs/README-before-user-guide.md).
