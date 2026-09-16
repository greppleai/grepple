---
name: delegated-research-with-ask
description: "Use for isolated, read-only source retrieval when a cheaper model can batch-read known file ranges or perform substantial multi-step, source-backed investigation. Never use it as a code-review or approval agent. Keep implementation, judgment, mutation, builds, and final verification in the calling agent."
---

# Delegated research with `grepple ask`

Use `grepple ask` as a read-only source-retrieval subagent, not as a reviewer or replacement for the coding agent. It can batch-read known ranges or perform messy exploration and return compact source-backed evidence while preserving the calling agent's context for implementation and judgment.

## Why use it

A normal unfamiliar-code investigation can consume many interactive turns and large amounts of context: locate ownership, search symbols, inspect declarations, follow calls, sift through many files, examine architecture, check an exact dependency version, and synthesize the evidence. `grepple ask` delegates that noisy loop to one isolated model invocation.

Benefits:

- **Context isolation:** tool calls, source excerpts, and intermediate reasoning stay in the delegated run and its JSONL log instead of filling the calling agent's context.
- **Cost-efficient delegation:** use a cheaper user-configured model for broad retrieval and evidence sifting while retaining the main model's context and capability for implementation, judgment, and verification.
- **Source-backed answers:** the research agent is instructed to cite repository paths and exact line ranges.
- **Purpose-built tools:** it receives typed tools for text search, exact navigation, structural search, graph queries, directory architecture, source scope, indexed refs and trees, and bounded batch file reads.
- **No shell exposure:** tools invoke Grepple internals directly; the delegated model gets no shell, generic argv command, mutation surface, or recursive `ask` capability.
- **Local and remote evidence:** one investigation can combine the current checkout with indexed repositories, branches, and tags.
- **Debuggability:** every run writes a semantic JSONL transcript containing tool schemas, completed model steps, tool calls/results, usage, answer, and errors without noisy token chunks.

## Use it when

Delegate when at least one of these applies:

- Several known files or ranges should be read together without spending one main-agent tool call per file.
- The question requires several search/navigation steps and a synthesized evidence handoff.
- Ownership, coupling, responsibilities, architecture, or change impact is unclear.
- A structural pattern must be found and then interpreted in source context.
- The answer spans multiple packages or repositories.
- An exact dependency branch or tag must be inspected remotely.
- The calling agent needs a compact source retrieval report before implementing a larger change.

Good examples:

- “Read these five implementation ranges and return them verbatim with their local anchors.”
- “Which package owns navigation resolution, what calls it, and what would a signature change affect?”
- “Compare our streaming integration with Fantasy v0.8.0 and cite both repositories.”
- “On the exact indexed selector, query the impact graph and summarize directory responsibilities without conflating graph edges with ownership.”
- “Explain why these directories depend on each other and identify the first source-backed relation.”

## Do not use it when

Prefer direct Grepple tools or a normal file read when:

- One literal search, outline, or exact `PATH:LINE` lookup answers the question.
- You already know only one small edit-ready range; read it directly instead.
- The task asks for code review, approval, defect hunting, or a second opinion. The main agent owns review and judgment.
- The requested operation mutates files, credentials, configuration, rules, indexes, or artifacts.
- A remote exact ref is not indexed and the conclusion must be version-specific. Ask may identify this gap, but it cannot manufacture missing evidence.
- Provider cost or latency is not justified by the question's complexity.

When local anchors are enabled, request known ranges through `read_file` and preserve every `HASH│LINE│content` row exactly in the handoff. If discovery used search or navigation, finish by batch-reading the selected local ranges; never replace anchored source rows with unanchored snippets. This lets the calling agent edit directly without another read. Do not invoke `ask` from inside another delegated `ask` run.

## Prerequisites

Check provider state when authentication is uncertain:

```bash
grepple ai-provider list
```

If needed, ask the user to complete provider authentication:

```bash
grepple ai-provider login codex                 # device OAuth
grepple ai-provider login copilot               # GitHub device flow
grepple ai-provider login anthropic             # API key
grepple ai-provider login anthropic-subscription # claude setup-token
grepple ai-provider login openai                # API key
grepple ai-provider login bedrock               # validate AWS credential chain
```

Select another service with `grepple ask --model <provider>/<model>`. The same prefixed value in `~/.grepple/grepple.json` `ask.model` is the user default. `--provider` remains available for unprefixed command-line model names. Ask logs default to enabled with `ask.logs.retentionPeriod: "7d"`; disable creation with `ask.logs.enabled: false`. Prefer a capable cheaper research model and override it only when an investigation needs a different cost/capability tradeoff.

## Recommended workflow

### 1. Write an answerable research brief

Include:

- the concrete question;
- local versus remote scope;
- an exact `OWNER/REPO`, branch, or tag when relevant;
- the evidence expected in the answer;
- distinctions the model must preserve, such as production versus tests or confirmed versus candidate navigation edges.

Avoid vague prompts such as “understand this codebase.”

### 2. Delegate with an overall deadline

Local example:

```bash
grepple ask --timeout-seconds 600 \
  'Explain how ask constructs and runs its typed research tools. Identify where schemas originate and cite exact source ranges.'
```

Remote exact-version example:

```bash
grepple ask --server http://127.0.0.1:8080 --timeout-seconds 600 \
  'In indexed charmbracelet/fantasy at tag v0.8.0, resolve the exact indexed selector first, then identify Agent.Stream and OnToolResultFunc with exact repository/path ranges.'
```

Impact example:

```bash
grepple ask \
  'Trace callers and callees of BuildNavigationGraphFromDocuments, explain its ownership boundary, and identify source-backed change risks. Distinguish resolved edges from candidates.'
```

There is no step-count cutoff. The overall timeout is the execution bound, so set it deliberately.

### 3. Evaluate the returned evidence

Check that the answer:

- cites exact paths and ranges;
- uses the requested repository/ref rather than silently falling back;
- discloses missing indexed refs, truncation, parse failures, or candidate navigation;
- distinguishes observed source facts from interpretation;
- actually answers the question rather than merely listing matches.

Treat candidate navigation edges and heuristic architecture findings as leads, not proof.

### 4. Verify before changing code

The delegated answer is a research handoff. Before editing:

- retrieve the smallest critical source ranges directly;
- verify any claim that controls the implementation decision;
- run normal tests, lint, builds, or runtime checks in the calling agent;
- do not cite the model's prose when source evidence is available.

## Reading the debug log

Every run prints a path such as:

```text
Ask log: ~/.grepple/ask-logs/20260915T184257.155470459Z-....jsonl
```

Use the log when the answer is missing, expensive, repetitive, or surprising. Important event types are:

- `session.start`: question, provider/model, system prompt, server, and complete typed tool metadata;
- `tool.call` / `tool.result`: exact structured research requests and evidence returned;
- `tool.cache`: whether a typed call was served from the invocation cache or shared an in-flight duplicate;
- `tool.timing`: call ID, full typed input, invocation/execution milliseconds, response bytes, failures, and cache state without duplicated response content;
- `research.universe`: whether local navigation, graph, and architecture tools created or reused one parsed source universe;
- `step.finish`: assembled response and reasoning content for a completed model step;
- `stream.finish`: finish reason and usage;
- `agent.finish` / `session.finish`: complete result and final answer;
- `session.performance`: total, LLM-facing, tool-invocation, actual tool-execution, and non-stream wall times plus deterministic per-tool aggregates;
- `agent.error` / `session.error`: provider or orchestration failure.

Logs omit OAuth credentials and authorization headers, but they can contain proprietary questions, source, and model reasoning. They are stored with mode `0600`; handle and remove them according to the repository's retention policy.

## Efficient prompting rules

- Ask one coherent research question per invocation.
- Request exact source ranges explicitly.
- Name the repository and desired version instead of saying “the dependency.”
- Tell the agent to resolve the indexed ref before making version-specific claims.
- Ask it to distinguish exact/import-resolved/context-resolved navigation from candidates.
- Ask for production-only conclusions only when the question truly excludes tests and fixtures.
- Prefer a compact conclusion plus evidence over a chronological narration of every tool call.
- If the answer is broad, narrow the next prompt rather than asking the same question again.

## Failure handling

- **No text answer:** inspect the JSONL log for repeated calls, tool errors, timeout, or provider finish reasons, then narrow the prompt or increase only the timeout.
- **Transient provider overload:** retry after a delay; do not change source or credentials in response.
- **Missing remote ref:** inspect indexed refs, request/index the exact version, and rerun. Do not accept default-branch evidence as proof about a tag.
- **Truncated tool result:** narrow repository paths, symbols, graph depth, or result limits.
- **Weak citation:** verify directly and rerun with the exact missing evidence requested.

## Handoff format

Preserve only what the implementation agent needs:

- concise conclusion;
- repository/ref and owning package;
- critical `PATH:START-END` evidence;
- confirmed versus candidate relations;
- completeness limitations;
- smallest next source location or implementation action.
