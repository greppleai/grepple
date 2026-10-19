import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createGuardHandler } from "./guard.mjs";

export { createGuardHandler } from "./guard.mjs";

export default function (pi: ExtensionAPI) {
  pi.on("tool_call", createGuardHandler());
}