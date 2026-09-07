import { readFileSync } from "node:fs";

export function loadConfig(file) {
  const raw = readFileSync(file, "utf8");
  return JSON.parse(raw);
}

export class ConsoleReporter {
  report(message) {
    console.log(`[report] ${message}`);
  }

  error(error) {
    console.error(error);
  }
}
