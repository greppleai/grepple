import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { chmod, mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const guard = join(dirname(fileURLToPath(import.meta.url)), 'grep-guard.mjs');
const input = '{"tool_name":"Bash","tool_input":{"command":"rg x ."}}';

test('guard prefers a working PATH CLI, falls back to plugin data, and fails open', async t => {
  if (process.platform === 'win32') { t.skip('shell stubs are Unix-only'); return; }
  const root = await mkdtemp(join(tmpdir(), 'grepple-guard-test-'));
  try {
    const system = join(root, 'system'), data = join(root, 'data');
    await mkdir(system);
    await mkdir(join(data, 'bin'), { recursive: true });
    const cli = join(system, 'grepple'), privateCLI = join(data, 'bin', 'grepple');
    const run = () => spawnSync(process.execPath, [guard, data], { input, encoding: 'utf8', env: { PATH: `${system}:/usr/bin:/bin` } });
    await writeFile(cli, '#!/bin/sh\nexit 2\n');
    await chmod(cli, 0o755);
    await writeFile(privateCLI, '#!/bin/sh\necho \'{"hookSpecificOutput":{"permissionDecision":"deny"}}\'\n');
    await chmod(privateCLI, 0o755);
    assert.equal(JSON.parse(run().stdout).hookSpecificOutput.permissionDecision, 'deny');
    await writeFile(cli, '#!/bin/sh\necho \'{"hookSpecificOutput":{"permissionDecision":"allow"}}\'\n');
    assert.equal(JSON.parse(run().stdout).hookSpecificOutput.permissionDecision, 'allow');
    await rm(privateCLI);
    await writeFile(cli, '#!/bin/sh\nexit 2\n');
    assert.equal(run().stdout, '');
  } finally { await rm(root, { recursive: true, force: true }); }
});
