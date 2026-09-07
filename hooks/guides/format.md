# gofmt formatting is auto-fixed

## Goal

Keep every Go file gofmt-clean with zero agent effort.

## How it works here

The Stop hook runs `gofmt -l .` before linting and rewrites any listed file
with `gofmt -w` automatically. The feedback note names the files it touched.

## What the agent must do

- Nothing to fix by hand — but if you still hold un-run edits or test
  expectations that read those files, re-run them against the formatted
  content.
- Never hand-format against gofmt, and never mix structural edits into a
  formatting pass.
- Do not reformat files outside the reported scope without need.

## Completion check

`gofmt -l .` prints nothing and the touched files' diffs contain no behavioral
change.
