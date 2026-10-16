import test from 'node:test';
import assert from 'node:assert/strict';
import { chmod, mkdtemp, mkdir, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { assetFor, expectedDigest, install } from './install-cli.mjs';

test('release asset names match supported platforms only', () => {
  for (const [platform, arch, expected] of [
    ['linux', 'x64', 'linux_amd64.tar.gz'],
    ['linux', 'arm64', 'linux_arm64.tar.gz'],
    ['darwin', 'x64', 'darwin_amd64.tar.gz'],
    ['darwin', 'arm64', 'darwin_arm64.tar.gz'],
    ['win32', 'x64', 'windows_amd64.zip'],
  ]) assert.equal(assetFor(platform, arch, 'v0.0.6'), `grepple_0.0.6_${expected}`);
  for (const [platform, arch] of [['win32', 'arm64'], ['freebsd', 'x64'], ['linux', 'ia32']]) {
    assert.throws(() => assetFor(platform, arch, 'v0.0.6'), /unsupported/);
  }
  assert.throws(() => assetFor('linux', 'x64', '../main'), /unexpected release tag/);
});

test('checksum manifest rejects absent, duplicate, or invalid digests', () => {
  const digest = 'a'.repeat(64);
  assert.equal(expectedDigest(`${digest}  grepple.tar.gz\n`, 'grepple.tar.gz'), digest);
  for (const manifest of ['', `${digest}  other`, `${digest}  grepple.tar.gz\n${digest}  grepple.tar.gz`, 'junk  grepple.tar.gz']) {
    assert.throws(() => expectedDigest(manifest, 'grepple.tar.gz'), /checksum/);
  }
});

test('existing plugin-private CLI persists PATH without a download', async t => {
  if (process.platform === 'win32') { t.skip('the Windows install uses PowerShell'); return; }
  const root = await mkdtemp(join(tmpdir(), 'grepple-claude-test-'));
  const previous = process.env.CLAUDE_ENV_FILE;
  const previousPath = process.env.PATH;
  process.env.PATH = '/usr/bin:/bin';
  try {
    const bin = join(root, 'bin');
    await mkdir(bin);
    const fake = join(bin, 'grepple');
    await writeFile(fake, '#!/bin/sh\nif [ "$1" = "--version" ]; then echo "grepple 0.0.6"; else echo \'{"hookSpecificOutput":{"permissionDecision":"deny"}}\'; fi\n');
    await chmod(fake, 0o755);
    const env = join(root, 'session.env');
    await writeFile(env, 'export TEST=one\n');
    process.env.CLAUDE_ENV_FILE = env;
    await install(root);
    const text = await readFile(env, 'utf8');
    assert.match(text, /^export TEST=one\nexport PATH=".*\/bin:\$PATH"\n$/);
  } finally {
    process.env.PATH = previousPath;
    if (previous === undefined) delete process.env.CLAUDE_ENV_FILE;
    else process.env.CLAUDE_ENV_FILE = previous;
    await rm(root, { recursive: true, force: true });
  }
});

test('refuses a symlinked plugin bin directory before downloading', async t => {
  if (process.platform === 'win32') { t.skip('symlink privileges vary on Windows'); return; }
  const root = await mkdtemp(join(tmpdir(), 'grepple-claude-link-'));
  const oldPath = process.env.PATH;
  try {
    await mkdir(join(root, 'external'));
    await symlink(join(root, 'external'), join(root, 'bin'));
    process.env.PATH = '/';
    await assert.rejects(install(root), /regular directory/);
  } finally {
    process.env.PATH = oldPath;
    await rm(root, { recursive: true, force: true });
  }
});
