// SessionStart bootstrap: install a checksum-verified release into plugin-private data.
// Never run a remote script or replace a user-managed executable.
import { createHash } from 'node:crypto';
import { existsSync } from 'node:fs';
import { appendFile, chmod, lstat, mkdtemp, mkdir, rename, rm, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const releaseRoot = 'https://github.com/greppleai/grepple/releases';
const maxArchiveBytes = 128 * 1024 * 1024;
const probeInput = '{"tool_name":"Bash","tool_input":{"command":"rg x ."}}';

export function assetFor(platform, arch, tag) {
  const os = { linux: 'linux', darwin: 'darwin', win32: 'windows' }[platform];
  const cpu = { x64: 'amd64', arm64: 'arm64' }[arch];
  if (!os || !cpu || (os === 'windows' && cpu !== 'amd64')) {
    throw new Error(`unsupported Grepple release platform: ${platform}/${arch}`);
  }
  if (!/^v\d+\.\d+\.\d+(?:[-+][\w.+-]+)?$/.test(tag)) throw new Error('unexpected release tag');
  return `grepple_${tag.slice(1)}_${os}_${cpu}.${os === 'windows' ? 'zip' : 'tar.gz'}`;
}

export function expectedDigest(manifest, asset) {
  const rows = manifest.trim().split(/\r?\n/).map(line => line.trim().split(/\s+/));
  const hits = rows.filter(([, name]) => name === asset);
  if (hits.length !== 1 || !/^[0-9a-f]{64}$/.test(hits[0][0])) throw new Error('invalid or missing release checksum');
  return hits[0][0];
}

async function fetchHTTPS(url, maxBytes) {
  for (let hops = 0; hops <= 5; hops++) {
    if (new URL(url).protocol !== 'https:') throw new Error('release redirect is not HTTPS');
    const response = await fetch(url, { redirect: 'manual', signal: AbortSignal.timeout(120000) });
    if ([301, 302, 303, 307, 308].includes(response.status)) {
      url = new URL(response.headers.get('location') || '', url).href;
      await response.body?.cancel();
      continue;
    }
    if (!response.ok) throw new Error(`release download failed (${response.status})`);
    const declared = Number(response.headers.get('content-length'));
    if (declared > maxBytes) throw new Error('release asset exceeds size limit');
    let length = 0;
    const parts = [];
    for await (const part of response.body) {
      length += part.length;
      if (length > maxBytes) throw new Error('release asset exceeds size limit');
      parts.push(part);
    }
    return Buffer.concat(parts);
  }
  throw new Error('too many release redirects');
}

function run(binary, args, options = {}) {
  const result = spawnSync(binary, args, { encoding: 'utf8', timeout: 30000, maxBuffer: 1024 * 1024, ...options });
  if (result.error || result.status !== 0) throw new Error(`${binary} failed: ${result.error?.message || result.stderr || result.status}`);
  return result.stdout.trim();
}

function supportsGuard(binary) {
  try {
    const result = JSON.parse(run(binary, ['claude-grep-guard'], { input: probeInput, timeout: 5000 }));
    return result?.hookSpecificOutput?.permissionDecision === 'deny';
  } catch { return false; }
}

function workingCLI(binary) {
  try { return run(binary, ['--version'], { timeout: 5000 }).startsWith('grepple '); }
  catch { return false; }
}

async function extract(archive, asset, stage, executable) {
  const archivePath = join(stage, asset);
  await writeFile(archivePath, archive);
  if (asset.endsWith('.zip')) {
    const script = join(dirname(fileURLToPath(import.meta.url)), 'extract-windows.ps1');
    run('powershell.exe', ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', script, archivePath, executable]);
  } else {
    const files = run('tar', ['-tzf', archivePath]);
    if (files !== 'grepple') throw new Error('unexpected release archive contents');
    run('tar', ['-xzf', archivePath, '-C', stage, 'grepple']);
  }
  if (!(await lstat(executable)).isFile()) throw new Error('release executable is not a regular file');
  if (!asset.endsWith('.zip')) await chmod(executable, 0o755);
}

async function persistPath(binDir) {
  if (!process.env.CLAUDE_ENV_FILE) return;
  let dir = binDir;
  if (process.platform === 'win32') dir = run('cygpath', ['-u', binDir]);
  if (/[\r\n]/.test(dir)) throw new Error('invalid plugin data path');
  const escaped = dir.replace(/[\\"$`]/g, '\\$&');
  await appendFile(process.env.CLAUDE_ENV_FILE, `export PATH="${escaped}:$PATH"\n`);
}

export function readinessMessage({ cliSource, guardSource }) {
  let guard = 'grep guard active';
  if (!guardSource) guard = 'grep guard OFF (no available CLI supports claude-grep-guard; update the CLI)';
  else if (cliSource === 'PATH' && guardSource === 'plugin data') {
    guard = 'grep guard active via plugin data; the PATH CLI lacks guard support, so update it';
  }
  return `Grepple: CLI ready (${cliSource}); ${guard}. For local repository discovery try grepple overview first (model-free; see coverage and real labels), then grepple tree --depth 1 and focused search. If overview is unavailable or metadata missing, use unfiltered tree/search; metadata selectors like --purpose work only on current local annotations.`;
}

export async function install(dataDir) {
  if (!dataDir || !isAbsolute(dataDir)) throw new Error('an absolute plugin data directory is required');
  const binDir = join(dataDir, 'bin');
  const binary = join(binDir, process.platform === 'win32' ? 'grepple.exe' : 'grepple');
  if (workingCLI('grepple')) { // Never replace a working user-managed CLI.
    const guardSource = supportsGuard('grepple') ? 'PATH' : (existsSync(binary) && supportsGuard(binary) ? 'plugin data' : null);
    return { cliSource: 'PATH', guardSource };
  }
  if (existsSync(binDir) && !(await lstat(binDir)).isDirectory()) throw new Error('plugin bin directory is not a regular directory');
  if (existsSync(binary)) {
    if (!(await lstat(binary)).isFile()) throw new Error('plugin CLI is not a regular file');
    if (!workingCLI(binary)) throw new Error('existing plugin CLI cannot start; remove its bin directory to reinstall');
    await persistPath(binDir);
    return { cliSource: 'plugin data', guardSource: supportsGuard(binary) ? 'plugin data' : null };
  }
  const platform = process.platform, arch = process.arch;
  assetFor(platform, arch, 'v0.0.0'); // Fail unsupported platforms before network access.
  const latest = JSON.parse((await fetchHTTPS('https://api.github.com/repos/greppleai/grepple/releases/latest', 1024 * 1024)).toString('utf8'));
  const tag = latest.tag_name;
  const asset = assetFor(platform, arch, tag);
  const manifest = (await fetchHTTPS(`${releaseRoot}/download/${tag}/checksums.txt`, 1024 * 1024)).toString('utf8');
  const expected = expectedDigest(manifest, asset);
  const archive = await fetchHTTPS(`${releaseRoot}/download/${tag}/${asset}`, maxArchiveBytes);
  if (createHash('sha256').update(archive).digest('hex') !== expected) throw new Error('release checksum mismatch');
  await mkdir(binDir, { recursive: true });
  if (!(await lstat(binDir)).isDirectory()) throw new Error('plugin bin directory is not a regular directory');
  const stage = await mkdtemp(join(binDir, '.grepple-stage-'));
  try {
    const stagedBinary = join(stage, platform === 'win32' ? 'grepple.exe' : 'grepple');
    await extract(archive, asset, stage, stagedBinary);
    if (!workingCLI(stagedBinary)) throw new Error('release CLI cannot start with current settings; installation left unchanged');
    if (!existsSync(binary)) await rename(stagedBinary, binary);
  } finally { await rm(stage, { recursive: true, force: true }); }
  await persistPath(binDir);
  return { cliSource: 'plugin data', guardSource: supportsGuard(binary) ? 'plugin data' : null };
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  install(process.env.CLAUDE_PLUGIN_DATA || process.argv[2])
    .then(status => { console.log(readinessMessage(status)); })
    .catch(error => {
      console.error(`Grepple plugin setup: ${error.message}`);
      console.log('Grepple: CLI setup failed; grep guard readiness unknown (hook fails open). Check Grepple installation before relying on the guard.');
    });
}
