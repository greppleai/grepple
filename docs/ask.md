# Delegated research with `grepple ask`

`grepple ask` runs a bounded, read-only internal research agent backed by a larger model. The agent can invoke Grepple recursively for search, navigation, GritQL, graph, architecture, boundary, source-scope, and indexed-repository inspection, plus a bounded local file reader. This trades model/API cost for fewer interactive investigation turns in the calling agent.

## Provider authentication

AI provider credentials are user-owned and never loaded from `grepple.json`:

```bash
grepple ai-provider list
grepple ai-provider login codex
grepple ai-provider logout codex
```

Codex login uses OpenAI's device authorization flow. The CLI prints the verification URL and one-time code and opens the browser when possible; use `--no-browser` to suppress browser launch. Credentials are atomically stored with mode `0600` in `~/.grepple/ai-providers.json`. Override the location with `GREPPLE_AI_CREDENTIALS` for isolated automation or testing.

The provider registry is intentionally provider-agnostic. Each provider owns login, logout, status, token refresh, and Fantasy language-model construction. Codex is the first registered provider. Its OAuth access token is sent only to the configured Codex API endpoint, with the account identifier required by that endpoint. Expiring tokens refresh automatically.

## Ask

Questions do not need shell quoting when they contain only ordinary words:

```bash
grepple ask Which package owns navigation resolution and what calls it?
grepple ask --model gpt-5.3-codex --max-steps 16 \
  Compare the parser lifecycle with the architecture single-parse path
```

Options:

- `--provider NAME` selects a registered provider (`codex` by default).
- `--model MODEL` selects the provider model (`gpt-5.3-codex` by default).
- `--max-steps N` bounds model/tool iterations from 1 to 30 (default 12).
- `--timeout-seconds N` bounds the whole run from 1 to 3600 seconds (default 600).

Human answers participate in Grepple's normal bounded-output and artifact-spill behavior.

## Tool safety and limits

The internal agent receives two tools:

1. `grepple` executes argv directly without a shell and caps captured output at 64 KiB. It can use read-only Grepple research commands, including local and remote navigation. Recursive `ask`, provider/login/logout commands, artifact deletion, anchor writes, and saved-rule mutations are rejected.
2. `read` accepts a repository-relative path and an optional line range. It rejects path escapes and symlink escapes, binary files, files larger than 256 KiB, and ranges larger than 1,000 lines.

The system prompt requires source-backed answers with repository/path:line evidence and instructs the model to narrow broad searches before retrieving bodies. Tool errors are returned to the model so it can recover within the remaining step budget.
