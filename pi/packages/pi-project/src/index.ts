import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

import { registerProjectTools } from "./tool.js";

export default function (pi: ExtensionAPI): void {
  registerProjectTools(pi);
}
