import assert from "node:assert/strict";
import { chmod, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { MAX_BYTES, checkCommand, createGuardHandler, decodeGuardResponse } from "./guard.mjs";

async function stub(root, name, source) {
  const path = join(root, name);
  await writeFile(path, `#!/usr/bin/env node\n${source}`);
  await chmod(path, 0o755);
  return path;
}

test("decodeGuardResponse accepts only the guard protocol", () => {
  assert.deepEqual(decodeGuardResponse(Buffer.from('{"blocked":false}')), { blocked: false });
  assert.deepEqual(decodeGuardResponse(Buffer.from('{"blocked":true,"reason":"use grepple"}')), { blocked: true, reason: "use grepple" });
  for (const value of [Buffer.from("bad"), Buffer.from('{"blocked":true}'), Buffer.alloc(MAX_BYTES + 1)]) {
    assert.equal(decodeGuardResponse(value), undefined);
  }
});

test("checkCommand blocks only a valid deny response and fails open otherwise", async () => {
  const root = await mkdtemp(join(tmpdir(), "grepple-pi-guard-"));
  try {
    const deny = await stub(root, "deny", 'process.stdin.resume(); process.stdin.on("end", () => console.log(JSON.stringify({blocked:true,reason:"use grepple"})));');
    const allow = await stub(root, "allow", 'process.stdin.resume(); process.stdin.on("end", () => console.log(JSON.stringify({blocked:false})));');
    const malformed = await stub(root, "malformed", 'process.stdin.resume(); process.stdin.on("end", () => console.log("not json"));');
    const slow = await stub(root, "slow", 'setTimeout(() => process.exit(0), 200);');
    assert.deepEqual(await checkCommand("rg x", { candidates: [deny] }), { blocked: true, reason: "use grepple" });
    assert.deepEqual(await checkCommand("echo x", { candidates: [allow] }), { blocked: false });
    assert.equal(await checkCommand("rg x", { candidates: [malformed] }), undefined);
    assert.equal(await checkCommand("rg x", { candidates: [slow], timeoutMs: 10 }), undefined);
    assert.equal(await checkCommand("rg x", { candidates: [join(root, "missing")] }), undefined);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("Earendil Pi tool-call policy blocks writes and limits reads", async () => {
  const root = await mkdtemp(join(tmpdir(), "grepple-pi-policy-"));
  try {
    const deny = await stub(root, "deny", 'process.stdin.resume(); process.stdin.on("end", () => console.log(JSON.stringify({blocked:true,reason:"Use the grepple CLI instead"})));');
    const handler = createGuardHandler({ candidates: [deny] });
    const context = { cwd: root };
    assert.deepEqual(await handler({ toolName: "bash", input: { command: "rg needle ." } }, context), {
      block: true,
      reason: "Use the grepple CLI instead",
    });
    assert.equal(await handler({ toolName: "read", input: { path: ".pi/skills/guard/SKILL.md" } }, context), undefined);
    assert.equal(await handler({ toolName: "read", input: { path: "AGENTS.md" } }, context), undefined);
    assert.deepEqual(await handler({ toolName: "read", input: { path: "README.md" } }, context), {
      block: true,
      reason: "Blocked: read is limited to AGENTS.md and SKILL.md files.",
    });
    assert.deepEqual(await handler({ toolName: "write", input: { path: "notes.md" } }, context), {
      block: true,
      reason: "Blocked: write is disabled in this Pi session.",
    });
    assert.equal(await handler({ toolName: "edit", input: { path: "notes.md" } }, context), undefined);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});