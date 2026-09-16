# Delegated research with `grepple ask`

`grepple ask` runs a bounded, read-only internal research agent, usually with a cheaper model than the main coding agent. The delegated model receives focused JSON tools that directly invoke Grepple's search, parser navigation, GritQL, graph, architecture, source-scope, indexed-tree, and file-reading APIs. It receives neither a shell nor a generic Grepple command surface. Its job is to sift through noisy, multi-file evidence and return a compact source-backed handoff, trading modest model/API cost for fewer interactive turns and a cleaner, more focused main-agent context.

## Provider authentication

AI provider credentials are user-owned and never loaded from repository configuration or `~/.grepple/grepple.json`:

```bash
grepple ai-provider list
grepple ai-provider login codex
grepple ai-provider logout codex
```

The built-in providers are:

| Provider | Authentication | Default model |
| --- | --- | --- |
| `codex` | OpenAI Codex device OAuth with automatic refresh | `gpt-5.3-codex` |
| `copilot` | GitHub device authorization plus Copilot subscription verification | `gpt-4.1` |
| `anthropic` | `ANTHROPIC_API_KEY`, `GREPPLE_ANTHROPIC_API_KEY`, or a hidden login prompt | `claude-sonnet-4-5-20250929` |
| `anthropic-subscription` | `CLAUDE_CODE_OAUTH_TOKEN`, `GREPPLE_ANTHROPIC_OAUTH_TOKEN`, or a hidden prompt for a token created by `claude setup-token` | `claude-sonnet-4-5-20250929` |
| `openai` | `OPENAI_API_KEY`, `GREPPLE_OPENAI_API_KEY`, or a hidden login prompt | `gpt-5.1` |
| `bedrock` | AWS SDK default credential chain and region configuration | `anthropic.claude-sonnet-4-5-20250929-v1:0` |

```bash
grepple ai-provider login copilot
grepple ai-provider login anthropic
grepple ai-provider login anthropic-subscription
grepple ai-provider login openai
AWS_PROFILE=research AWS_REGION=us-east-1 grepple ai-provider login bedrock

grepple ask --model copilot/gpt-4.1 'Question'
grepple ask --model anthropic/claude-sonnet-4-5-20250929 'Question'
```

Codex and Copilot login print the verification URL and one-time code immediately while authorization polling continues. `--no-browser` suppresses browser launch. Static credentials entered interactively are read without terminal echo. Credentials are atomically stored with mode `0600` in `~/.grepple/ai-providers.json`; override the location with `GREPPLE_AI_CREDENTIALS` for isolated automation or testing. Merely using an API-key environment variable does not copy it into the store. Logout removes only the stored provider entry; an active environment variable continues to authenticate that provider. Concurrent provider updates are serialized so one login cannot overwrite another.

Anthropic subscription access uses Anthropic's user-created setup token rather than collecting account passwords. Availability and permitted use remain controlled by Anthropic's subscription terms. Bedrock does not copy AWS secrets into Grepple: `login bedrock` validates the current AWS SDK credential chain and records only a validation marker, while `list` revalidates the active chain before reporting `logged-in`. Run the appropriate AWS SSO login or configure environment/shared-profile credentials first; Grepple logout does not remove external AWS credentials.

Each provider owns login, logout, status, refresh, and Fantasy language-model construction. API credentials are sent only to that provider's configured endpoint. Copilot exchanges the stored GitHub token for a short-lived Copilot token before model construction; Codex refreshes expiring OAuth credentials automatically.

## Ask

Questions do not need shell quoting when they contain only ordinary words:

```bash
grepple ask Which package owns navigation resolution and what calls it?
grepple ask --model codex/gpt-5.3-codex \
	Compare the parser lifecycle with the architecture single-parse path
```

Options:

- `--model [PROVIDER/]MODEL` selects both provider and model when prefixed, for example `copilot/gpt-4.1`. An unprefixed command-line model uses `--provider`, or Codex when `--provider` is omitted.
- `--provider NAME` remains available for unprefixed model names. A conflicting `--provider` and prefixed `--model` is rejected. When both are omitted, `ask.model` selects the provider and model, then Codex's provider default is the final fallback.
- `--server URL` selects the remote Grepple service available to tools with an indexed-repository selector. When omitted, normal Grepple server configuration applies.
- `--timeout-seconds N` bounds the whole run from 1 to 3600 seconds (default 600).

Set a non-secret per-user model default independently of credentials:

```json
{
  "ask": {
    "model": "copilot/gpt-4.1",
    "logs": {
      "enabled": true,
      "retentionPeriod": "7d"
    }
  }
}
```

Command-line `--model` always takes precedence. Prefixing the single `ask.model` value keeps provider and model inseparable and prevents an alias from being sent to the wrong service. Repository-owned `grepple.json` files cannot select the AI model. Prefer a capable cheaper model for broad research and override it only when an investigation needs a different cost/capability tradeoff.

Human answers participate in Grepple's normal bounded-output and artifact-spill behavior.

## Debug logs

Logging defaults to enabled with a seven-day retention period. An enabled invocation immediately prints an `Ask log:` path to stderr and writes `grepple-ask-log-v1` JSONL under `~/.grepple/ask-logs/`. Set `ask.logs.enabled` to `false` to stop creating logs. `ask.logs.retentionPeriod` accepts positive durations such as `7d`, `168h`, or `30m`.

At the beginning of every ask invocation, Grepple removes managed `.jsonl` logs whose modification time is older than the configured retention period—even when new logging is disabled. Unrelated files and unrecognized names in the directory are retained. `GREPPLE_ASK_LOG_DIR` selects another directory for isolated automation.

The log records the question, selected provider/model, system and tool prompts, assembled model and reasoning content for each completed step, tool calls and complete results, per-call `tool.cache` hit/shared status, shared `research.universe` creation/reuse, usage, final answer, and errors. It intentionally omits noisy per-token stream chunks. Partial logs remain useful if a run is interrupted. Files use mode `0600` and the directory uses `0700`. Logs deliberately omit OAuth credentials and raw authorization headers, but they can contain sensitive questions, model reasoning, and retrieved source.

## Tool safety and limits

The internal agent receives a small set of typed, read-only tools:

- `search_code`: literal or regex source search with `count`, `files`, and `snippets` result shapes.
- `navigate_code`: exact `PATH:LINE` declaration retrieval with bounded callers and callees.
- `structural_search`: native `gritql-v1` syntax matching.
- `inspect_architecture`: local or exact indexed-repository directory architecture, symbol resolution, relation evidence, and responsibility summaries.
- `query_graph`: local or exact indexed-repository bounded callers, callees, dependencies, dependents, and impact queries.
- `explain_sources`: source classifications, exclusions, and completeness.
- `repository_refs`: exact indexed default-branch, branch, and tag selectors for one source repository.
- `repository_tree`: bounded indexed-repository path discovery.
- `read_file`: bounded local or indexed-repository source ranges and structural outlines.

Each tool has a purpose-specific JSON schema with only the relevant options. Tools call Grepple's search, parser, graph, architecture, GritQL, source-scope, and remote HTTP APIs directly. The agent receives no generic argv tool, command parser, executable subprocess, or shell.

One invocation owns a source/config/server-identified research session. Successful identical typed calls reuse byte-identical evidence, and concurrent duplicates share one execution. Cache status is exposed through tool-response metadata and `tool.cache` log events; errors are not cached, and a canceled waiter does not cancel shared work governed by the overall ask timeout. The cache is in-memory and never survives the invocation.

One invocation owns a source/config/server-identified research session. Successful identical typed calls reuse byte-identical evidence, and concurrent duplicates share one execution. Cache status is exposed through tool-response metadata and `tool.cache` log events; errors are not cached, and a canceled waiter does not cancel shared work governed by the overall ask timeout. The cache is in-memory and never survives the invocation.

Local `navigate_code`, `query_graph`, and `inspect_architecture` also share a lazily initialized source universe when their normalized selected paths and `max_files` scope agree. Each selected file is parsed into one caller-owned `parser.Document`; outlines, exact declaration context, related navigation, graph traversal, and directory architecture borrow those documents and one resolved navigation analysis until the ask session closes them. Different effective scopes receive separate universes, and cold versus reused tool evidence remains byte-identical.
