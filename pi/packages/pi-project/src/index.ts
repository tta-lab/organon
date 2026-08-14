import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { registerProjectTool } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerProjectTool(pi);
}
