---
name: remote-grep-and-search
description: "Use whenever the target is not in the local working directory: searching across repositories or a GitHub org, searching or reading a repo you have not cloned, fetching a remote file or line range, browsing a remote repo's tree, or answering a recurring org-wide code question. Do not git clone just to read, and do not use grep/rg/find. For the local working directory, use local-grep-and-search."
---

# grepple — remote / multi-repo search

Beyond the local working directory, `grepple` can search every repository a
shard/router has **indexed**, and fetch files/trees from them without cloning. The
remote side is **org/repo agnostic**: any repository the server watches is
addressable as `OWNER/REPO`. For local search mechanics (flags, globs, structural
output, paging, `--outline`) see the `local-grep-and-search` skill — this skill
covers only what is remote-specific.

## Local-first: opt into the remote

**Search runs locally by default.** grepple only contacts the shard/router when you
opt in with `--remote` (`-R`) or by passing `--server URL` (which implies
`--remote`). A configured `GREPPLE_SERVER` / config `server` just supplies the URL to
use *when* you opt in — it does not make every search hit the network.

```bash
grepple "TODO" src                         # local only (default)
grepple -R "TODO" src                      # local + configured remote, combined
grepple -s https://grepple.example "TODO"    # explicit server (implies --remote)
```

When remote search runs, local and remote results are **combined**. If the CWD is
inside a Git worktree, grepple derives `owner/name` from the remote and excludes that
repo from the remote search to avoid duplicates. `--local` forces local-only;
`--local` + `--remote` is an error.

`get`, `tree`, and `rules` are **inherently remote** — they always talk to the
server (they call `/public/raw`, `/public/tree`, `/public/rules`).

## Server configuration & precedence

When a remote call happens (`--remote`/`--server`, or `get`/`tree`/`rules`), grepple
resolves the server URL in this order (first wins):

1. `--server URL` / `-s URL` flag
2. `GREPPLE_SERVER` environment variable
3. `./grepple.json` in the current directory — `{ "server": "https://..." }`
4. `~/.grepple/config.json` — same shape
5. `http://127.0.0.1:8787` (final fallback)

Authenticated servers use `grepple login` (OAuth device flow); the token is stored in
`~/.grepple/config.json` and sent as a bearer header, and silently refreshed.

> Tip: to discover which repos a server indexes, run `grepple repos` (see below).
> It hits `/public/repos` with your login token, so it works even when the root
> endpoint is not exposed.

## Remote-relevant search flags

Everything in the `local-grep-and-search` flag table also applies; these matter most
for remote:

| Flag | Meaning |
| --- | --- |
| `-R`, `--remote` | Also query the configured remote shard/router (default: local only) |
| `-s URL`, `--server URL` | Remote shard/router URL (implies `--remote`) |
| `--repo PATTERN` | Restrict to matching indexed repos (repeatable); matches the full `OWNER/REPO` name, so a bare repo name may not match. Also lets shards skip repos they don't hold. |
| `-c`, `--count` | Per-repository match tally, aggregated server-side over the *complete* match set — the cheapest "where does this concentrate?" probe |
| `--files-with-matches` | Paths of files whose contents match (`grep -l`), no bodies |
| `--limit N` | Cap returned files (**default 20**, **max 100 per page** — `--limit 0` or a larger N returns the first 100; page further with `--skip N --limit 100`). Keep it small: the shard only parses the files it returns |

## Token-efficient cross-repo workflow

Probe cheaply before pulling bodies:

1. **Locate** — `grepple PATTERN --remote --count` → per-repo tally, hottest first
   (tiny fixed payload no matter how many matches).
2. **Narrow** — re-run scoped to the hot repo(s): `grepple PATTERN --repo OWNER/REPO`.
3. **List** — `grepple PATTERN --repo OWNER/REPO --files-with-matches` → matching paths.
4. **Read** — `grepple get OWNER/REPO PATH --lines A:B`, or a bounded
   `grepple PATTERN --repo OWNER/REPO --limit 5`.

```bash
grepple -R "useEffect" "src/*.tsx"                          # local + remote combined
grepple "httpRoute" --count                                # 1) locate (per-repo tally)
grepple "httpRoute" --repo OWNER/REPO                      # 2) narrow to the hot repo
grepple "httpRoute" --repo OWNER/REPO -l "**/values.yaml"  #    filename glob within a repo
grepple "httpRoute" --repo OWNER/REPO --files-with-matches # 3) list matching files (grep -l)
```

(`--count`, `--repo`, and `-l/--files` all imply/trigger remote when a server is
configured *and* you pass `--remote`; add `-R` if you are not already opting in.)

## List indexed repos (`repos`)

Discover the exact `OWNER/REPO` names a server has indexed — no throwaway
`--count` probe or root-endpoint curl needed. Hits `/public/repos` with your
login token, one repo per line (sorted); pass an optional case-insensitive
SUBSTRING to filter, or `--json` for full detail (shard, ref, head, indexedAt).

```bash
grepple repos                     # every indexed OWNER/REPO, one per line
grepple repos helm                # only names containing "helm"
grepple repos --json              # structured: repo + shard + ref + head + indexedAt
```

`repos` flags: `--json`, `-s/--server`. Exits non-zero when nothing matches.

## Fetch a raw file (`get`)

Read a file — README, config, source — from an indexed repo without cloning:

```bash
grepple get OWNER/REPO README.md
grepple get OWNER/REPO path/to/file.go --lines 20:50   # only lines 20–50
grepple get OWNER/REPO package.json --json             # metadata + content as JSON
grepple get OWNER/REPO path/to/File.java -O            # structural OUTLINE of a remote file
```

`get` flags: `--lines A:B` (inclusive 1-based range), `-O/--outline`, `--json`,
`-s/--server`.

## Explore a repo (`tree`)

```bash
grepple tree OWNER/REPO                # top 2 levels
grepple tree OWNER/REPO charts --depth 3
grepple tree OWNER/REPO --json         # machine-readable tree
```

`tree` flags: `--depth N` (default 2), `--json`, `-s/--server`.

## Recommended workflow for "read file X from repo Y"

1. Confirm the repo's indexed name (org/repo agnostic):
   `grepple repos <substring>`
2. Locate the file if the exact path is unknown:
   `grepple --repo OWNER/REPO -l "**/README*"`  or browse: `grepple tree OWNER/REPO --depth 3`
3. Fetch it: `grepple get OWNER/REPO <path>` (add `--lines A:B` for a slice).

## Predefined searches (rules)

A **rule** is a saved search whose result is *materialized per repository* and
refreshed automatically whenever a repo is reindexed — like a Prometheus recording
rule, but over the code corpus. Fetching a rule's results is O(read), so it is the
cheap way to answer recurring org-wide questions (e.g. "which repos use a given
GitHub Action?") without re-scanning everything each time.

```bash
# 'add' takes the same options as search (PATTERN, GLOBs, --regex, -i,
# --repo/--exclude-repo). Mode is 'count' (per-repo files/matches, default) or
# 'files' (matching paths, via -l/--files).
grepple rules add --name "Uses Checkout" "uses: actions/checkout" ".github/workflows/*.yml"
grepple rules add --id go-mods -l "module" "**/go.mod"        # files mode

grepple rules list                    # id, mode, name
grepple rules get  uses-checkout      # the definition (JSON)
grepple rules results uses-checkout   # per-repo tally (add --json for machine output)
grepple rules rm   uses-checkout
```

`results` is one row per repo (`OWNER/REPO<TAB>N files<TAB>M matches`, plus matching
paths in files mode), ordered by repo name (no ranking). Results are eventually
consistent: each repo's row reflects its last indexed commit and creating a rule
backfills across all repos in the background, so poll `results` shortly after.

## Gotchas

- `--repo shortname` may return nothing — prefer the full `OWNER/REPO`.
- **Remote search skips the repo you are standing in** (the CWD's `OWNER/REPO` is excluded so local+remote combined searches don't double-report it). Sharp edge: `--repo OWNER/REPO --remote` from *inside that same repo* returns nothing at all — remote excludes it, and the local side can't match the `--repo` filter because local result paths are relative. To query the *indexed* content of the repo you have checked out, run from a different directory, or use `grepple get`/`tree` (always remote).
- Piping `grepple ... | head` can exit 141 (SIGPIPE); the fetch/search still succeeded.
- A configured server is **not** contacted unless you pass `--remote`/`--server`
  (or use `get`/`tree`/`rules`). If a search seems to miss indexed repos, you likely
  forgot `-R`.
- For very broad remote queries the index may hit its result cap and return an
  *incomplete* (but valid) sample, printing `note: results truncated …`. Narrow with
  `--repo`, tighten the pattern, or `--count` first — don't assume you saw everything.
- `--count` tallies come straight from the index and can include a few binary/generated
  files a content scan skips, so a count probe can read slightly higher than what
  paged content searches return (e.g. 667 counted vs 666 listable) — a small delta is
  expected, not a bug.
- For plain local search mechanics (flags, globs, structural output, paging,
  `--outline`), see the **`local-grep-and-search`** skill.
