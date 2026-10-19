---
name: semantic-commits
description: Use when asked to commit, split commits, or suggest a commit message in this repository. Choose semantic Conventional Commits and stage only intended work.
---

# Semantic commits

1. Commit only when explicitly requested. Inspect `git status --short --branch`, the working diff, and the staged diff; preserve unrelated or pre-existing edits. Stage only intended files or hunks (`git add -p` for mixed files). Do not use `git add -A` unless the request clearly includes every change.
2. Run relevant checks from `AGENTS.md`, inspect `git diff --cached` and `git diff --cached --check`, and keep secrets, credentials, local artifacts, and unrelated generated changes out of the commit.
3. Write a Conventional Commit subject: `<type>(<scope>): <imperative summary>` (scope optional). Prefer `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, or `chore`; use a concise lowercase imperative subject without a trailing period. For an incompatible change use `!` and a `BREAKING CHANGE:` footer. Explain non-obvious motivation in the body.
4. Split independent changes into separate commits when useful, but keep coupled code, tests, and metadata consistent. Do not amend, rebase, push, or rewrite history unless explicitly asked. Report commit hash(es), checks run, and any remaining uncommitted changes.

This is a repository-local **agent instruction**, not a Grepple CLI or plugin-distributed skill. Do not add it to `.agents/skills`, `internal/agentskills`, or any CLI packaging manifest.
