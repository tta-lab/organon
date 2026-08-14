import { withFileMutationQueue } from "@earendil-works/pi-coding-agent";
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

// Re-exported so tests can exercise the mutation queue participation with the
// same helper the execute path uses.
export { withFileMutationQueue };
