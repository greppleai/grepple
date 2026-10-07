# Grepple

Grepple helps you find text, inspect code, and follow source relationships in a repository. It runs locally without a server. Searches return results in deterministic path order; narrow the files or query when you need a smaller answer.

## Install

### Download the latest release (Linux and macOS)

The installer selects your OS and CPU architecture, downloads the latest published
GitHub release, verifies its SHA-256 checksum, and installs `grepple` into
`~/.local/bin`. It needs `curl`, `tar`, and `sha256sum` (Linux) or `shasum` (macOS);
no Go toolchain, C compiler, or `sudo` is needed.

```sh
installer="$(mktemp)"
curl -fsSL https://raw.githubusercontent.com/greppleai/grepple/main/install.sh -o "$installer" &&
  sh "$installer"
rm -f "$installer"
export PATH="$HOME/.local/bin:$PATH"
grepple --version
```

You can inspect the downloaded script before running it. Add the `export PATH`
line to your shell profile (`~/.bashrc` or `~/.zshrc`) to make it persistent. The
installer does not change shell profiles or saved login settings. Rerun it to
upgrade; it replaces the CLI only after the archive passes verification.

From a checkout, run `sh install.sh`. To use another writable directory already
on PATH, run `sh install.sh --bin-dir /your/bin` (or set `GREPPLE_BIN_DIR`). If a
different `grepple` appears first on PATH, adjust PATH order; `command -v grepple`
shows which executable your shell selects. Supported platforms are Linux/macOS
on amd64 and arm64. Windows users can download the `.zip` from
[GitHub Releases](https://github.com/greppleai/grepple/releases) and put
`grepple.exe` in a directory on PATH.

### Build from source

You need Go 1.27 or later and a C compiler for the bundled Tree-sitter parsers:

```bash
git clone https://github.com/greppleai/grepple.git
cd grepple
make build
./bin/grepple --version
```

Use `make install` to install the locally built CLI. Run `grepple --help` for the current command and flag list.

## Set up your coding agent

Install Grepple's skills for your chosen harness:

```sh
grepple setup pi
grepple setup claude
grepple setup opencode
grepple setup codex
```

Run the command for the agent you use. Skills are downloaded from the **same
release tag as your binary**, never `main` or `latest`. Setup replaces managed
Grepple skills and removes recognized retired copies, while preserving unrelated
skills, agent settings, and credentials. Restart/reload your agent afterward.

Use `--dry-run` to preview, `--project` for the current project instead of user
skills, or `grepple setup --list` to see current and historical owned names.
Source/development builds require an explicit matching checkout, for example
`grepple setup pi --source-dir /path/to/grepple`. See [agent setup](docs/agent-setup.md)
for install locations, compatibility checks, cleanup and ownership rules.

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

### What an agent sees

For example, an agent can locate a declaration, read its anchored source, and follow the suggested next point. These are shortened excerpts from this repository; line numbers and hashes change as code changes:

```text
$ grepple --outline internal/directorymeta/repository.go
internal/directorymeta/repository.go  go
81-91   func  ReadRepository
93-113  func  loadRepository

$ grepple --at internal/directorymeta/repository.go:81
internal/directorymeta/repository.go
rL1│81│func ReadRepository(root string) (Repository, error) {
buS│82│    repository, err := loadRepository(root)
...
Next points (code navigation):
  → loadRepository  internal/directorymeta/repository.go:93-113  call:82
```

`HASH│LINE│content` rows identify source for follow-up reads or [anchored edits](docs/write.md); `Next points` link to related declarations. For automation, `grepple -F 'ReadRepository' internal/directorymeta --json --limit 1` returns results **and** a copyable continuation command (abridged fields shown):

```json
{
  "results": [{"path": "internal/directorymeta/repository.go", "matches": [{"line": 81, "text": "func ReadRepository(root string) (Repository, error) {"}]}],
  "metadata": {
    "page": {"skip": 0, "limit": 1, "returned": 1, "complete": false},
    "nextCommand": "grepple search -F --skip 1 --limit 1 --json ReadRepository internal/directorymeta"
  }
}
```

Here `complete: false` means more result files are available; the agent can run `nextCommand` rather than assuming the first page is exhaustive.

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
