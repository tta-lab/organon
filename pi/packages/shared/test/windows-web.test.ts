import { existsSync } from "node:fs";
import { describe, expect, it } from "vitest";

import { resolveBinaryPath } from "../src/binary.js";
import { detectPlatform } from "../src/platform.js";

describe("Windows Pi Web native package", () => {
  it.skipIf(process.platform !== "win32" || process.arch !== "x64")(
    "resolves the test-owned web.exe without PATH lookup",
    () => {
      expect(detectPlatform("web")).toEqual({ os: "win32", arch: "x64" });
      const executable = resolveBinaryPath("web");
      expect(executable.endsWith("\\bin\\web.exe")).toBe(true);
      expect(existsSync(executable)).toBe(true);
    },
  );
});
