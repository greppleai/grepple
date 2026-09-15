# Delegated research with `grepple ask`

`grepple ask` runs a bounded, read-only internal research agent, usually with a cheaper model than the main coding agent. The delegated model receives focused JSON tools that directly invoke Grepple's search, parser navigation, GritQL, graph, architecture, source-scope, indexed-tree, and file-reading APIs. It receives neither a shell nor a generic Grepple command surface. Its job is to sift through noisy, multi-file evidence and return a compact source-backed handoff, trading modest model/API cost for fewer interactive turns and a cleaner, more focused main-agent context.

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
grepple ask --model gpt-5.3-codex \
	Compare the parser lifecycle with the architecture single-parse path
```

Options:

- `--provider NAME` selects a registered provider (`codex` by default).
- `--model MODEL` selects the provider model. When omitted, `ai.model` from `~/.grepple/grepple.json` is used, then the provider default (`gpt-5.3-codex`).
- `--server URL` selects the remote Grepple service available to tools with an indexed-repository selector. When omitted, normal Grepple server configuration applies.
- `--timeout-seconds N` bounds the whole run from 1 to 3600 seconds (default 600).

Set a non-secret per-user model default independently of credentials:

```json
{
  "ai": {
    "model": "gpt-5.6-luna"
  }
}
```

Command-line `--model` always takes precedence. Repository-owned `grepple.json` files cannot select the AI model. Prefer a capable cheaper model as the user default for broad research; override it only when an investigation needs a different cost/capability tradeoff.

Human answers participate in Grepple's normal bounded-output and artifact-spill behavior.

## Debug logs

Every invocation immediately prints an `Ask log:` path to stderr and writes `grepple-ask-log-v1` JSONL under `~/.grepple/ask-logs/`. The log records the question, selected provider/model, system and tool prompts, assembled model and reasoning content for each completed step, tool calls and complete results, usage, final answer, and errors. It intentionally omits noisy per-token stream chunks. Partial logs remain useful if a run is interrupted. Files use mode `0600` and the directory uses `0700`.

Logs deliberately omit OAuth credentials and raw authorization headers, but they can contain sensitive questions, model reasoning, and retrieved source. Remove them according to your retention policy. `GREPPLE_ASK_LOG_DIR` selects another directory for isolated automation.

## Tool safety and limits

The internal agent receives a small set of typed, read-only tools:

- `search_code`: literal or regex source search with `count`, `files`, and `snippets` result shapes.
- `navigate_code`: exact `PATH:LINE` declaration retrieval with bounded callers and callees.
- `structural_search`: native `gritql-v1` syntax matching.
- `inspect_architecture`: local directory architecture, symbol resolution, and relation evidence.
- `query_graph`: bounded callers, callees, dependencies, dependents, and impact queries.
- `explain_sources`: source classifications, exclusions, and completeness.
- `repository_refs`: exact indexed default-branch, branch, and tag selectors for one source repository.
- `repository_tree`: bounded indexed-repository path discovery.
- `read_file`: bounded local or indexed-repository source ranges and structural outlines.

Each tool has a purpose-specific JSON schema with only the relevant options. Tools call Grepple's search, parser, graph, architecture, GritQL, source-scope, and remote HTTP APIs directly. The agent receives no generic argv tool, command parser, executable subprocess, or shell.

The system prompt requires source-backed answers with repository/path:line evidence and instructs the model to narrow broad searches before retrieving bodies. There is no step-count cutoff; the overall timeout remains the execution bound. Tool errors are returned to the model so it can recover.
