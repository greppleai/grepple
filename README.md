# Grepple

**Find code, read exact source, and follow relationships—locally, without a server.** Grepple searches text and supported syntax in a checkout. An indexed service is optional for remote repositories.

[Get started](#get-started) · [Install](#install) · [For coding agents](#for-coding-agents) · [Documentation](docs/README.md)

## Get started

Run these from a repository root (the example paths below are in this checkout):

```sh
grepple overview                         # First: bounded current-label vocabulary and coverage; no model
grepple tree internal/render             # Expand only a likely directory
grepple -F 'OutlineWithSelectors' internal/render  # Literal search
grepple --outline internal/render/outline.go       # Declarations
grepple --at internal/render/outline.go:58-69        # Read exact lines
```

For metadata-filtered discovery, opt into **bounded** `grepple init --only-directory internal/render` (model-backed), check `grepple verify`, then run the model-free `grepple overview` to see actual labels before choosing `grepple tree --purpose Implementation internal/render` or `grepple --purpose Implementation -F 'OutlineWithSelectors' internal/render`. Missing, invalid, or stale annotations never expand a filtered result. [Schema, refresh, and limitations](docs/directory-metadata.md).

`-F` searches literal text; without it, search patterns are regular expressions. Text files remain searchable even without a parser. Results are in deterministic path order, not relevance order. Narrow your path or use `--count-summary` before interpreting a short page as complete.

| Need | Start with |
| --- | --- |
| Find matching lines | `grepple -F 'Name' --line-only PATH` |
| List matching files / count matches | `grepple -F 'Name' --files-with-matches PATH` / `--count-summary` |
| Find a declaration | `grepple --outline FILE` or `grepple graph resolve --symbol NAME PATH` |
| Follow calls | `grepple graph callers --at FILE:LINE PATH` or `graph callees` |
| Map directory relations | `grepple architecture directory PATH` |
| Find syntax shapes | `grepple grit --query-file checks.grit PATH` |
| Learn by example | `grepple examples` or [worked examples](examples/README.md) |

### From an outline to verified source

Human `--outline` includes local parser-verified selectors where available. An illustrative row looks like `OutlineWithSelectors @func:58-69`; copy its **current** token as a separate argument:

```sh
grepple --at internal/render/outline.go @func:58-69
```

The verified form checks the current kind and both bounds; `--at FILE:START-END` reads literal inclusive lines instead. Unverified outline rows retain `START-END<TAB>KIND<TAB>NAME`. `--outline --json` keeps structured output without selector decoration. For matched lines, `--line-only` may show scope hints, but its decorated human rows are **not edit-ready**: re-read clean source with `--at` before editing. See [verified selectors](docs/at-verified-scope-selectors.md) and [transactional writes](docs/write.md).

## Install

### Published CLI (Linux/macOS)

Install the latest published CLI with one command (review [the script](install.sh) first if you prefer):

```sh
curl -fsSL https://raw.githubusercontent.com/greppleai/grepple/main/install.sh | sh
```

Verify it directly at its installation path with `$HOME/.local/bin/grepple --version`.

The [installer](install.sh) chooses Linux/macOS amd64 or arm64, checks the release archive's SHA-256, and installs to `~/.local/bin`. It needs `curl`, `tar`, and `sha256sum` or `shasum`. It never changes your shell profile; add `~/.local/bin` to your PATH for future shells. Windows binaries are on [GitHub Releases](https://github.com/greppleai/grepple/releases).

For a custom destination, append `-s -- --bin-dir /your/bin` after `sh`. A `curl | sh` pipeline can mask download errors; if you need a reliable exit status, run `bash -o pipefail -c 'curl -fsSL https://raw.githubusercontent.com/greppleai/grepple/main/install.sh | sh'` instead.

### Build from source

Requires Go 1.27+ and a C compiler for the bundled Tree-sitter parsers:

```sh
git clone https://github.com/greppleai/grepple.git
cd grepple
make build
./bin/grepple --version
```

`make install` installs the locally built CLI. Check `command -v grepple` if a different version appears first on PATH; `grepple --help` lists current commands.

## For coding agents

Claude Code users: install [Node.js 18+](https://nodejs.org/) for plugin hooks, then in a Claude Code session run:

```text
/plugin install grepple --marketplace greppleai/grepple
```

This adds the Grepple marketplace and nine source-owned skills (Claude Code 2.1.275+), including `grepple-overview-first` to check local annotation coverage and vocabulary at session start. On the next session start, a hook uses an existing working CLI or downloads a checksum-verified release for Linux/macOS (amd64/arm64) or Windows (amd64) into Claude's plugin data; it never replaces a user-managed binary. A grep guard redirects Bash `grep`/`rg`/`find` and built-in Grep/Glob to Grepple once the CLI supports it. Try `/grepple:grepple-overview-first`. [Installation, security, and compatibility details](docs/agent-setup.md#claude-code-plugin).

For Pi, OpenCode, Codex, or a binary-version-pinned Claude Code skill installation instead:

```sh
grepple setup pi       # Or: claude, opencode, codex
grepple setup --list   # Show owned skill names
```

`grepple setup` installs skills matched to the **binary's release**, not the moving `main` branch. Use `--dry-run` to inspect changes and `--project` for project-local skills; a development binary needs `--source-dir /path/to/grepple`. Restart your agent after installing. For Claude Code, choose **either** the plugin or `grepple setup claude` to avoid duplicate skills. [Setup and ownership details](docs/agent-setup.md).

A reliable agent loop: **tree → narrow search/outline → verify with `--at` → inspect callers if changing behavior → edit with fresh anchors**. Local clean source reads return `HASH│LINE│content` rows for `grepple write`; search annotations and remote source are not local edit authority. Human output can be capped or spilled; search JSON reports paging/completeness and a continuation command when available. Check those before claiming absence. [Output contracts](docs/output-contracts.md) · [Source selection](docs/source-scope.md) · [Write guide](docs/write.md).

## Optional and advanced

- **Remote indexed repositories:** use a compatible service and an explicit `OWNER/REPO[@REF]` selector. Local search requires no service. [Remote indexing](docs/versioned-indexing.md).
- **AI-assisted research:** `grepple ask` needs a configured provider; it is read-only. [Ask setup](docs/ask.md).
- **CLI traces:** opt in through user-owned settings. [Telemetry](docs/cli-telemetry.md).
- **Reference and design notes:** browse the [documentation index](docs/README.md) and [supported file types](docs/file-type-support.md).
