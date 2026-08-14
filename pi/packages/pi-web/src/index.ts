import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { registerWebTool } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerWebTool(pi);
}
