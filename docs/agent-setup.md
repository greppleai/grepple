# Agent setup

`grepple setup pi|claude|opencode|codex` installs the complete active Grepple skill
bundle for one selected harness. `--agent NAME` is an alternative to the positional
name. Grepple must be on the agent's PATH; this command installs instructions and
supporting documents, not the agent itself, hooks, extensions, or MCP servers.
It never edits agent settings, tools, instructions files, or credentials.

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
   does not leave checkout-relative links broken. The write skill's reference is
   a copy of `docs/write.md`; keep them byte-identical.
3. Update `internal/agentskills/catalog.json`: current skills and relative files
   with SHA-256 digests. Remove retired skills only from this active catalog.
   Preserve historical legacy fingerprints for recognizing pre-marker installs.
4. Update repository metadata/checksums. Rebuild and run
   `go test ./internal/agentskills ./internal/cli/setup ./internal/cli`.
   Tests cover catalog/directory parity, append-only history, resource integrity,
   migration, retirement, conflict preservation, cancellation and rollback.
5. Publish the catalog, ownership registry and skills in the **same immutable tag**
   as the binary. Never publish a binary that points to a different skill revision.

Harness discovery references: [Pi skills](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/skills.md),
[Claude Code skills](https://code.claude.com/docs/en/skills),
[OpenCode skills](https://opencode.ai/docs/skills/), and
[Codex skills](https://developers.openai.com/codex/skills).
