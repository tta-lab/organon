import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { registerOgTool } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerOgTool(pi);
}
