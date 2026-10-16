import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { isAbsolute, join } from 'node:path';

// Fail open if installation fails; never block unrelated work due to a broken hook.
const dataDir = process.env.CLAUDE_PLUGIN_DATA || process.argv[2];
const binary = dataDir && isAbsolute(dataDir) ? join(dataDir, 'bin', process.platform === 'win32' ? 'grepple.exe' : 'grepple') : '';
const chunks = [];
let size = 0;
for await (const chunk of process.stdin) {
  size += chunk.length;
  if (size > 1024 * 1024) process.exit(0);
  chunks.push(chunk);
}
const input = Buffer.concat(chunks);
for (const command of ['grepple', ...(binary && existsSync(binary) ? [binary] : [])]) {
  const result = spawnSync(command, ['claude-grep-guard'], {
    input, encoding: 'utf8', timeout: 10000, maxBuffer: 1024 * 1024,
  });
  if (!result.error && result.status === 0) {
    if (result.stdout) process.stdout.write(result.stdout);
    break;
  }
}
