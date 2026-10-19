import { existsSync, statSync } from "node:fs";
import { spawn } from "node:child_process";
import { isAbsolute, join } from "node:path";

export const MAX_BYTES = 1024 * 1024;
export const TIMEOUT_MS = 10_000;

export function decodeGuardResponse(output) {
  if (!Buffer.isBuffer(output) || output.length > MAX_BYTES) return undefined;
  try {
    const value = JSON.parse(output.toString("utf8"));
    if (!value || typeof value !== "object" || typeof value.blocked !== "boolean") return undefined;
    if (value.blocked && (typeof value.reason !== "string" || value.reason.length === 0)) return undefined;
    return value.blocked ? { blocked: true, reason: value.reason } : { blocked: false };
  } catch {
    return undefined;
  }
}

function localCandidate(cwd) {
  const candidate = join(cwd, "bin", process.platform === "win32" ? "grepple.exe" : "grepple");
  try {
    return statSync(candidate).isFile() ? candidate : undefined;
  } catch {
    return undefined;
  }
}

export function candidatesFor(cwd, env = process.env) {
  const configured = env.GREPPLE_BIN;
  return [...new Set([
    typeof configured === "string" && isAbsolute(configured) && existsSync(configured) ? configured : undefined,
    "grepple",
    localCandidate(cwd),
  ].filter(Boolean))];
}

function invoke(binary, input, { timeoutMs, spawnProcess }) {
  return new Promise(resolve => {
    let settled = false;
    let timedOut = false;
    const output = [];
    let outputSize = 0;
    const finish = value => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(value);
    };
    let child;
    const timer = setTimeout(() => {
      timedOut = true;
      child?.kill();
      finish(undefined);
    }, timeoutMs);
    try {
      child = spawnProcess(binary, ["grep-guard"], { stdio: ["pipe", "pipe", "ignore"], shell: false });
    } catch {
      finish(undefined);
      return;
    }
    child.on("error", () => finish(undefined));
    child.stdout.on("data", chunk => {
      outputSize += chunk.length;
      if (outputSize > MAX_BYTES) {
        child.kill();
        finish(undefined);
        return;
      }
      output.push(chunk);
    });
    child.on("close", code => {
      if (timedOut || code !== 0) return finish(undefined);
      finish(decodeGuardResponse(Buffer.concat(output)));
    });
    child.stdin.on("error", () => finish(undefined));
    child.stdin.end(input);
  });
}

// checkCommand invokes the host-neutral CLI guard. Undefined is intentionally
// fail-open: callers must block only a fully valid blocked response.
export async function checkCommand(command, options = {}) {
  if (typeof command !== "string") return undefined;
  const input = Buffer.from(JSON.stringify({ command }), "utf8");
  if (input.length > MAX_BYTES) return undefined;
  const timeoutMs = options.timeoutMs ?? TIMEOUT_MS;
  const spawnProcess = options.spawnProcess ?? spawn;
  for (const binary of options.candidates ?? candidatesFor(options.cwd ?? process.cwd(), options.env)) {
    const response = await invoke(binary, input, { timeoutMs, spawnProcess });
    if (response !== undefined) return response;
  }
  return undefined;
}

const allowedReadNames = new Set(["AGENTS.md", "SKILL.md"]);

// createGuardHandler implements the policy independently of a Pi package so
// regression tests can exercise it against Earendil Pi's documented event shape.
export function createGuardHandler(options = {}) {
  return async (event, ctx) => {
    if (event?.toolName === "read") {
      const path = event.input?.path;
      const name = typeof path === "string" ? path.split("/").pop() : undefined;
      if (!name || !allowedReadNames.has(name)) {
        return { block: true, reason: "Blocked: read is limited to AGENTS.md and SKILL.md files." };
      }
      return undefined;
    }
    if (event?.toolName === "write") {
      return { block: true, reason: "Blocked: write is disabled in this Pi session." };
    }
    if (event?.toolName !== "bash") return undefined;
    try {
      const response = await checkCommand(event.input?.command, { ...options, cwd: ctx.cwd });
      return response?.blocked ? { block: true, reason: response.reason } : undefined;
    } catch {
      return undefined;
    }
  };
}