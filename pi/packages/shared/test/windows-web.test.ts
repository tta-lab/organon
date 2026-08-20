import { execFile } from "node:child_process";
import { createRequire } from "node:module";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { promisify } from "node:util";

import { discoverAndLoadExtensions } from "@earendil-works/pi-coding-agent";
import { describe, expect, it } from "vitest";

import { resolveBinaryPath } from "../src/binary.js";
import { detectPlatform } from "../src/platform.js";

const execFileAsync = promisify(execFile);

describe("Windows Pi Web native package", () => {
  it.skipIf(
    process.platform !== "win32" ||
      process.arch !== "x64" ||
      !process.env.PI_WINDOWS_WEB_INSTALL_ROOT,
  )("loads the installed consumer and executes its resolved web.exe", async () => {
    const installRoot = process.env.PI_WINDOWS_WEB_INSTALL_ROOT!;
    const consumerRequire = createRequire(join(installRoot, "windows-smoke.mjs"));
    const consumerPackage = dirname(consumerRequire.resolve("@tta-lab/pi-web/package.json"));
    const agentDir = mkdtempSync(join(tmpdir(), "organon pi web agent "));
    try {
      const loaded = await discoverAndLoadExtensions([consumerPackage], installRoot, agentDir);
      expect(loaded.errors).toEqual([]);
      expect(loaded.extensions.some((entry) => entry.tools.has("web_search"))).toBe(true);

      expect(detectPlatform("web")).toEqual({ os: "win32", arch: "x64" });
      const executable = resolveBinaryPath("web", {
        resolve: (specifier) => consumerRequire.resolve(specifier),
      });
      expect(executable.endsWith("\\bin\\web.exe")).toBe(true);
      const { stdout } = await execFileAsync(executable, ["--help"], { cwd: installRoot });
      expect(stdout).toContain("Search the web");
    } finally {
      rmSync(agentDir, { recursive: true, force: true });
    }
  });
});
