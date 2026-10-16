import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { chmod, mkdtemp, mkdir, readFile, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { assetFor, expectedDigest, install, readinessMessage } from './install-cli.mjs';

const startup = join(dirname(fileURLToPath(import.meta.url)), 'install-cli.mjs');
const stubCLI = active => `#!/bin/sh\ncase "$1" in\n  --version) echo "grepple 0.0.6" ;;\n  claude-grep-guard) ${active ? `echo '{"hookSpecificOutput":{"permissionDecision":"deny"}}'` : 'exit 2'} ;;\n  *) exit 2 ;;\nesac\n`;

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
    const status = await install(root);
    assert.deepEqual(status, { cliSource: 'plugin data', guardSource: 'plugin data' });
    const text = await readFile(env, 'utf8');
    assert.match(text, /^export TEST=one\nexport PATH=".*\/bin:\$PATH"\n$/);
  } finally {
    process.env.PATH = previousPath;
    if (previous === undefined) delete process.env.CLAUDE_ENV_FILE;
    else process.env.CLAUDE_ENV_FILE = previous;
    await rm(root, { recursive: true, force: true });
  }
});
test('startup reports active, inactive, and plugin-private fallback without replacing PATH CLI', async t => {
  if (process.platform === 'win32') { t.skip('shell stubs are Unix-only'); return; }
  const root = await mkdtemp(join(tmpdir(), 'grepple-claude-status-'));
  const previousPath = process.env.PATH;
  const system = join(root, 'system');
  const privateBin = join(root, 'bin');
  try {
    await mkdir(system);
    await mkdir(privateBin);
    const pathCLI = join(system, 'grepple');
    const privateCLI = join(privateBin, 'grepple');
    process.env.PATH = `${system}:/usr/bin:/bin`;
    await writeFile(pathCLI, stubCLI(false));
    await chmod(pathCLI, 0o755);

    const inactive = await install(root);
    assert.deepEqual(inactive, { cliSource: 'PATH', guardSource: null });
    assert.match(readinessMessage(inactive), /grep guard OFF.*update the CLI/);

    await writeFile(privateCLI, stubCLI(true));
    await chmod(privateCLI, 0o755);
    const fallback = await install(root);
    assert.deepEqual(fallback, { cliSource: 'PATH', guardSource: 'plugin data' });
    assert.match(readinessMessage(fallback), /active via plugin data.*update it/);
    assert.equal(await readFile(pathCLI, 'utf8'), stubCLI(false));

    await writeFile(pathCLI, stubCLI(true));
    const active = await install(root);
    assert.deepEqual(active, { cliSource: 'PATH', guardSource: 'PATH' });
    assert.match(readinessMessage(active), /grep guard active.*grepple tree --depth 1/);
    const session = spawnSync(process.execPath, [startup, root], { encoding: 'utf8', env: { ...process.env, CLAUDE_PLUGIN_DATA: root } });
    assert.equal(session.status, 0);
    assert.equal(session.stdout.trim().split('\n').length, 1);
    assert.match(session.stdout, /grep guard active/);

    const failure = spawnSync(process.execPath, [startup, 'relative'], { encoding: 'utf8', env: { ...process.env, CLAUDE_PLUGIN_DATA: '' } });
    assert.equal(failure.status, 0); // Startup failure must still reach Claude as context.
    assert.match(failure.stdout, /CLI setup failed; grep guard readiness unknown/);
    assert.match(failure.stderr, /absolute plugin data directory/);
  } finally {
    process.env.PATH = previousPath;
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
