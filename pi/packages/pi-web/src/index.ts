import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { registerWebTools } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerWebTools(pi);
}
