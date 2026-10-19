# Agent setup

`grepple setup pi|claude|opencode|codex` installs the complete active Grepple skill
bundle for one selected harness. `--agent NAME` is an alternative to the positional
name. Grepple must be on the agent's PATH; this command installs instructions and
supporting documents, not the agent itself, hooks, extensions, or MCP servers.
It never edits agent settings, tools, instructions files, or credentials.

Claude Code users can alternatively install the [Grepple plugin](#claude-code-plugin)
from its marketplace; it uses the same source-owned skills, without running `grepple setup`.

## Locations

| Agent | Default user skills | `--project` skills under the current directory |
| --- | --- | --- |
| Pi | `~/.pi/agent/skills` | `.pi/skills` |
| Claude Code | `~/.claude/skills` | `.claude/skills` |
| OpenCode | `~/.config/opencode/skills` | `.opencode/skills` |
| Codex | `~/.agents/skills` | `.agents/skills` |

Pi respects an absolute `PI_CODING_AGENT_DIR`; OpenCode respects an absolute
`XDG_CONFIG_HOME`. Project installation ignores these user-directory overrides.
Project skills may require your harness's normal project-trust approval. Restart
or reload the agent after installing. Pi and OpenCode also discover shared
`.agents/skills` locations, so installing both a shared Codex copy and a native
copy can expose duplicate names; keep their Grepple versions aligned or keep only
one location. Setup does not silently modify another harness's directories.

```sh
grepple setup codex --dry-run
grepple setup --agent claude --project
grepple setup --list
```

## Pi grep guard extension

This checkout ships an opt-in [Earendil Works Pi](https://github.com/earendil-works/pi) extension at `extensions/pi-grepple/`; it is deliberately outside `.pi/` and is not auto-discovered. `grepple setup pi` installs skills only and does not install or enable extensions. Build a development CLI, then test it explicitly:

```sh
make build
pi --extension extensions/pi-grepple/index.ts
```

The extension blocks Pi `bash` calls that invoke `find`, `grep`, `rg`, `fd`, and related local-search programs, with the same shell-aware classifier and `# grep-guard:allow` exception as the Claude plugin. It permits Pi `read` only for requested paths named exactly `AGENTS.md` or `SKILL.md`, blocks all Pi `write` calls, and leaves `edit` and other non-requested tools alone. To auto-load it for a project, copy the directory to that project's `.pi/extensions/` and reload Pi. See the extension [README](../extensions/pi-grepple/README.md) for its machine protocol, bounds, remote-backend behavior, and removal instructions.

## Claude Code plugin

The plugin provides nine skills and two hooks. It requires Node.js 18+ on Claude
Code's PATH (including native Claude Code installations) to run the hooks.
In Claude Code 2.1.275 or later, one command in a Claude Code session registers
this repository's marketplace and installs its `grepple` plugin:

```text
/plugin install grepple --marketplace greppleai/grepple
```

For older Claude Code versions, register the marketplace and install separately
from a shell (or use the equivalent `/plugin marketplace add` and `/plugin install`
commands inside Claude Code):

```sh
claude plugin marketplace add greppleai/grepple
claude plugin install grepple@grepple
```

After installation, start or reload a Claude Code session. The `SessionStart`
hook checks for a working `grepple` on PATH. If absent, it downloads the **latest
published release** for Linux/macOS amd64 or arm64, or Windows amd64. It selects
the platform archive, checks SHA-256 against `checksums.txt`, rejects unexpected
archive members, and installs into `${CLAUDE_PLUGIN_DATA}/bin` without replacing
your existing binary, changing shell profiles, or requiring sudo. On Unix and
Windows Git Bash, it adds that directory to Claude's Bash-tool PATH via
`CLAUDE_ENV_FILE`. It does not install the CLI during `claude plugin install`:
SessionStart is the earliest supported automatic hook. Windows PowerShell tool
sessions do not inherit this Bash PATH adjustment; add the installed directory
to your PATH separately. Windows extraction uses PowerShell; Git Bash's
`cygpath` is needed for the Bash PATH adjustment. Unsupported platforms or
failed downloads leave the session unchanged and report an error. At each
`SessionStart`, the hook checks the available CLI for `claude-grep-guard` support
and supplies Claude one readiness line (active, inactive, or setup failed),
plus a first-search hint. Run `grepple --version` in Claude's shell to check
which CLI the agent will use.

A `PreToolUse` hook denies the built-in Grep/Glob tools and shell invocations of
`grep`, `rg`, `find`, `fd`, `ag`, `ack`, `locate`, and their variants. It returns
Grepple alternatives; ordinary shell arguments and quoted data are not blocked.
For a genuinely necessary shell exception append `# grep-guard:allow` to the
command. The guard fails open if its CLI cannot run. **The published CLI must
include `claude-grep-guard`** for this hook to enforce anything: older binaries
leave the guard inactive until a new release is published. If a PATH CLI works,
the installer does not replace it; an existing plugin-private CLI may still
run the guard when the PATH CLI lacks support. SessionStart probes the actual
hook capability, not version strings or plugin commits, and reports when the
CLI needs an upgrade. Plugin updates do not silently upgrade an existing
working CLI. If you prefer no automatic executable download or command blocking,
use `grepple setup claude` instead of the plugin.

Check `/plugin` and try `/grepple:grepple-local-grep-and-search`. Claude can
also select skills automatically. The marketplace lives at
`.claude-plugin/marketplace.json`; its source is `./.agents`, whose manifest is
`.agents/.claude-plugin/plugin.json`. All nine canonical
`.agents/skills/grepple-*/SKILL.md` files are discovered without duplicate skill
copies. To test a local checkout without registering a marketplace, run
`claude --plugin-dir /path/to/grepple/.agents`. Maintainers can run
`claude plugin validate .` and `claude plugin validate .agents` from the checkout.

Use either the plugin **or** `grepple setup claude`, not both: they expose the same
skills through different installation paths and can duplicate names. The plugin
follows the marketplace revision; `grepple setup claude` pins skills to the exact
CLI release and validates the checksums. Update the CLI and plugin together when
commands change. The plugin manifest intentionally omits a fixed version so
Claude Code uses the marketplace Git revision for updates (its validator notes
the missing version as a warning). To try an unpublished local marketplace, use
`claude plugin marketplace add /path/to/grepple` before
`claude plugin install grepple@grepple`; this affects your Claude Code settings.

## Version compatibility

Released binaries normalize their exact version to a Git tag (`0.1.0` becomes
`v0.1.0`). Setup downloads from
`https://raw.githubusercontent.com/greppleai/grepple/<TAG>/`, without redirects,
implicit cookies, or authorization. It does not clone, shell out to Git, or fall
back to `main`, `latest`, or another release if a tag/file is missing.

The downloaded ownership registry and catalog must exactly match the versions
embedded in the binary. Every skill file and bundled reference is SHA-256 checked
against that catalog before destination changes. Missing or incompatible files
fail closed; existing skills remain intact. Files are bounded to 1 MiB and the
bundle to 16 MiB, with per-request and overall deadlines and interrupt handling.
Checksums establish consistency with the trusted repository/tag, not independent
publisher authentication. Published tags must remain immutable.

Development, dirty, and `git describe` builds have no exact released skill bundle
and fail rather than guessing. Use an explicit checkout for development:

```sh
make build
./bin/grepple setup pi --source-dir . --dry-run
./bin/grepple setup pi --source-dir .
```

The checkout must match the binary's embedded catalog and file checksums. Rebuild
after updating the catalog. This override is clearly reported as
`local-development` and is not a claim of release-tag compatibility. Use the same
built CLI on the agent's PATH after testing; a different PATH binary can have a
different command contract.

## Ownership, migration, and safety

- All active skill directories and frontmatter names start with `grepple-`.
- `internal/agentskills/owned-skills.txt` is append-only. It includes original
  unprefixed names, including the retired `delegated-research-with-ask` skill.
  A missing active skill is retired, not forgotten. Cleanup checks only these
  exact names, never a prefix wildcard or arbitrary other skill directories.
- Each installed directory contains `.grepple-owned.json`, recording origin,
  source identity and file hashes. Repeat setup updates unchanged managed copies
  and removes unchanged managed skills that are no longer active.
- Pre-marker legacy copies are adopted/removed only when their sole `SKILL.md`
  matches a recorded historical digest. Unrecognized historical copies are
  preserved with a warning. Identical current source copies may also be adopted.
- Unmanaged active-name collisions, local changes to managed files, extra files,
  symlinks, non-regular files and symlinked destination parents are refused.
  Move customized copies somewhere safe and rerun; there is no destructive force
  flag. Unrelated skills are not touched.
- `--dry-run` downloads, validates and reports, but creates no directories, locks,
  or ownership markers. It does not mutate the repository or an agent profile.
- Setup serializes writers with a directory lock, prevalidates destinations,
  stages outside the discoverable skills root, checks snapshots before changing
  each directory, and rolls back installation failures. Recovery staging is
  retained on installation errors. A leftover lock after a crash must be removed
  only after checking that no setup is running. This is not a crash-proof journal.

## Maintaining the bundle

1. Add new skill names to the **end** of `owned-skills.txt`; never remove, reorder,
   or reuse any historical entry. Keep retired entries forever.
2. Keep active skills in `.agents/skills/<grepple-name>/` with matching frontmatter.
   Bundle referenced supporting files beneath the skill directory so installation
   does not leave checkout-relative links broken. The write skill is self-contained
   and documents only quoted `::grepple` heredoc transactions.
3. Update `internal/agentskills/catalog.json`: current skills and relative files
   with SHA-256 digests. Remove retired skills only from this active catalog.
   Preserve historical legacy fingerprints for recognizing pre-marker installs.
4. Update repository metadata/checksums. Rebuild and run
   `go test ./internal/agentskills ./internal/cli/setup ./internal/cli`.
   Tests cover catalog/directory parity, append-only history, resource integrity,
   migration, retirement, conflict preservation, cancellation and rollback.
5. Publish the catalog, ownership registry and skills in the **same immutable tag**
   as the binary. Never publish a binary that points to a different skill revision.
6. Before publishing plugin hooks that depend on new CLI commands, ship a CLI
   release with those commands. Plugin users with an older working CLI must
   upgrade it explicitly; otherwise the guard fails open. `make test` runs
   both Go and Node.js hook tests, and `claude plugin validate .agents` checks
   the Claude plugin manifest and hook metadata.

Harness discovery references: [Pi skills](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/skills.md),
[Claude Code skills](https://code.claude.com/docs/en/skills),
[OpenCode skills](https://opencode.ai/docs/skills/), and
[Codex skills](https://developers.openai.com/codex/skills).
