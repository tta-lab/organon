import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { registerReadEditTools } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerReadEditTools(pi);
}
