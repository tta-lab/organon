import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { describe, expect, it } from "vitest";

import srcExtension from "../../pi-src/src/index.js";

function withPlatform<T>(platform: NodeJS.Platform, arch: NodeJS.Architecture, run: () => T): T {
  const originalPlatform = Object.getOwnPropertyDescriptor(process, "platform");
  const originalArch = Object.getOwnPropertyDescriptor(process, "arch");
  if (!originalPlatform || !originalArch) {
    throw new Error("process platform descriptors unavailable");
  }
  Object.defineProperty(process, "platform", { value: platform });
  Object.defineProperty(process, "arch", { value: arch });
  try {
    return run();
  } finally {
    Object.defineProperty(process, "platform", originalPlatform);
    Object.defineProperty(process, "arch", originalArch);
  }
}

describe("extension startup", () => {
  it("rejects Windows x64 while registering src", () => {
    const pi = { registerTool() {}, on() {} } as unknown as ExtensionAPI;
    expect(() => withPlatform("win32", "x64", () => srcExtension(pi))).toThrow(/not supported/);
  });
});
