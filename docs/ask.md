# Ask research

`grepple ask "QUESTION"` runs the read-only source research agent against the current checkout. The command is part of the normal `grepple` CLI and appears in `grepple --help`. It requires a configured, authenticated AI provider; use `grepple ai-provider --help` for provider login and selection.

```sh
grepple ask --help
grepple ask --provider codex --model MODEL "Where is this symbol used?"
grepple ask --server https://grepple.example "Compare indexed repository usage"
```

`--provider` and `--model` override stored preferences; `--timeout-seconds` bounds the complete session (default 600; allowed 1–3600). `--server` configures the service for typed remote tools, while a tool's exact `OWNER/REPO[@REF]` input selects an indexed checkout. No repository is selected implicitly. Global source-scope flags such as `--no-repo-config`, `--no-config-ignore`, and `--production-only` apply to local discovery.

The agent uses bounded, source-backed reads, search, navigation, and structural tools; it must cite repository/path:line evidence and cannot mutate source. Grepple's ask preferences enable session logging by default with seven-day retention. Logs may contain prompts, source evidence, and tool responses; configure `ask.logs.enabled` and `ask.logs.retentionPeriod` in user settings if that is inappropriate for your environment.