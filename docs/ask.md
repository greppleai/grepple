# Delegated research with `grepple ask`

`grepple ask` runs a bounded, read-only internal research agent backed by a larger model. The agent can invoke Grepple recursively for search, navigation, GritQL, graph, architecture, boundary, source-scope, and indexed-repository inspection, plus a bounded local file reader. This trades model/API cost for fewer interactive investigation turns in the calling agent.

## Provider authentication

AI provider credentials are user-owned and never loaded from repository configuration or `~/.grepple/grepple.json`:

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
- `--model MODEL` selects the provider model. When omitted, `ai.model` from `~/.grepple/grepple.json` is used, then the provider default (`gpt-5.3-codex`).
- `--max-steps N` bounds model/tool iterations from 1 to 30 (default 12).
- `--timeout-seconds N` bounds the whole run from 1 to 3600 seconds (default 600).

Set a non-secret per-user model default independently of credentials:

```json
{
  "ai": {
    "model": "gpt-5.6-luna"
  }
}
```

Command-line `--model` always takes precedence. Repository-owned `grepple.json` files cannot select the AI model.

Human answers participate in Grepple's normal bounded-output and artifact-spill behavior.

## Debug logs

Every invocation immediately prints an `Ask log:` path to stderr and writes `grepple-ask-log-v1` JSONL under `~/.grepple/ask-logs/`. The log records the question, selected provider/model, system and tool prompts, assembled model and reasoning content for each completed step, tool calls and complete results, usage, final answer, and errors. It intentionally omits noisy per-token stream chunks. Partial logs remain useful if a run is interrupted. Files use mode `0600` and the directory uses `0700`.

Logs deliberately omit OAuth credentials and raw authorization headers, but they can contain sensitive questions, model reasoning, and retrieved source. Remove them according to your retention policy. `GREPPLE_ASK_LOG_DIR` selects another directory for isolated automation.

## Tool safety and limits

The internal agent receives two tools:

1. `grepple` executes argv directly without a shell and caps captured output at 64 KiB. Its tool description includes selection guidance, confidence/completeness caveats, argument rules, and examples for counts, matching files, outlines, exact `--at`, related traversal, GritQL, graph queries, architecture, source explanation, and remote `search`/`get`/`tree`. Recursive `ask`, provider/login/logout commands, artifact deletion, anchor writes, and saved-rule mutations are rejected.
2. `read` accepts a repository-relative path and an optional line range. It rejects path escapes and symlink escapes, binary files, files larger than 256 KiB, and ranges larger than 1,000 lines.

The system prompt requires source-backed answers with repository/path:line evidence and instructs the model to narrow broad searches before retrieving bodies. Tool errors are returned to the model so it can recover within the remaining step budget.
