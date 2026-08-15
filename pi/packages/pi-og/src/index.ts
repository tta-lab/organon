import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { registerOgTools } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerOgTools(pi);
}
