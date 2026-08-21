import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { describe, expect, it } from "vitest";

import ogExtension from "../../pi-og/src/index.js";
import projectExtension from "../../pi-project/src/index.js";
import srcExtension from "../../pi-src/src/index.js";
import webExtension from "../../pi-web/src/index.js";

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
  it("accepts Windows x64 for Pi Web", () => {
    const pi = { registerTool() {}, on() {} } as unknown as ExtensionAPI;
    expect(() => withPlatform("win32", "x64", () => webExtension(pi))).not.toThrow();
  });

  it.each([
    ["project", projectExtension],
    ["src", srcExtension],
    ["og", ogExtension],
  ] as const)("rejects Windows x64 while registering %s", (_name, extension) => {
    const pi = { registerTool() {}, on() {} } as unknown as ExtensionAPI;
    expect(() => withPlatform("win32", "x64", () => extension(pi))).toThrow(/pi-web only/);
  });
});
