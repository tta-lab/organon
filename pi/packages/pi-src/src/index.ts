import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { createTakeoverHandlers } from "./takeover.js";
import { registerSrcTool } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerSrcTool(pi);
  const takeover = createTakeoverHandlers(pi);
  pi.on("session_start", () => {
    takeover.onSessionStart();
  });
  pi.on("session_shutdown", () => {
    takeover.onSessionShutdown();
  });
}
