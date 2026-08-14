import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { describe, expect, it } from "vitest";

import ogExtension from "../../pi-og/src/index.js";
import projectExtension from "../../pi-project/src/index.js";
import srcExtension from "../../pi-src/src/index.js";
import webExtension from "../../pi-web/src/index.js";

function onUnsupportedPlatform<T>(run: () => T): T {
  const original = Object.getOwnPropertyDescriptor(process, "platform");
  if (!original) {
    throw new Error("process.platform descriptor is unavailable");
  }
  Object.defineProperty(process, "platform", { value: "win32" });
  try {
    return run();
  } finally {
    Object.defineProperty(process, "platform", original);
  }
}

describe("extension startup", () => {
  it.each([
    ["web", webExtension],
    ["project", projectExtension],
    ["src", srcExtension],
    ["og", ogExtension],
  ] as const)("rejects unsupported platforms while registering %s", (_name, extension) => {
    const pi = { registerTool() {}, on() {} } as unknown as ExtensionAPI;
    expect(() => onUnsupportedPlatform(() => extension(pi))).toThrow(/not supported/);
  });
});
