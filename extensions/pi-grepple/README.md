# Grepple Earendil Pi grep guard

This is a distributable [Earendil Works Pi](https://github.com/earendil-works/pi) extension. It is deliberately outside `.pi/` and is **not** auto-discovered from this checkout. It blocks built-in `bash` calls that invoke legacy local-search programs (`find`, `grep`, `rg`, `fd`, and related variants) and returns Grepple's canonical alternative. It allows `read` only when the requested path has the exact basename `AGENTS.md` or `SKILL.md`, blocks every built-in `write` call, and leaves `edit` and custom tools unchanged.

## Requirements and activation

Use Earendil Works Pi 1.3.0 or later and a Grepple CLI that supports `grep-guard` (this checkout does after `make build`). Test without installing it:

```sh
pi --extension extensions/pi-grepple/index.ts
```

To enable automatic project-local loading, copy the directory into your target project and restart Pi (or run `/reload`):

```sh
mkdir -p /path/to/project/.pi/extensions
cp -R extensions/pi-grepple /path/to/project/.pi/extensions/
```

The extension resolves its classifier in this order: an existing absolute `GREPPLE_BIN`, `grepple` on `PATH`, then `bin/grepple` in the active project. It sends `{"command":"…"}` to `grepple grep-guard` over stdin and accepts only `{"blocked":false}` or `{"blocked":true,"reason":"…"}`. Command input and captured output are limited to 1 MiB; the classifier has a 10-second deadline. It intentionally fails open when the binary is missing, exits unsuccessfully, times out, or returns malformed data. The classifier runs in Pi's local orchestrator process even when Pi's tool backend is remote.

Append `# grep-guard:allow` to a shell command only when a legacy search program is genuinely required. To deactivate an installed project copy, remove `/path/to/project/.pi/extensions/pi-grepple/` and restart Pi (or use `/reload`).

The extension emits no logs or telemetry and forwards only the guard's canonical denial reason to Pi.