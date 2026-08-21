import { execFile } from "node:child_process";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { promisify } from "node:util";

import { describe, expect, it } from "vitest";

import { resolveBinaryPath } from "../src/binary.js";
import { detectPlatform } from "../src/platform.js";

const execFileAsync = promisify(execFile);

describe("Windows Pi Web native package", () => {
  it.skipIf(
    process.platform !== "win32" ||
      process.arch !== "x64" ||
      !process.env.PI_WINDOWS_WEB_INSTALL_ROOT,
  )("resolves the installed native package and executes web.exe --help", async () => {
    const installRoot = process.env.PI_WINDOWS_WEB_INSTALL_ROOT!;
    const consumerRequire = createRequire(join(installRoot, "windows-smoke.mjs"));
    const nativePackage = "@tta-lab/pi-web-win32-x64";
    const nativeRoot = dirname(consumerRequire.resolve(`${nativePackage}/package.json`));

    expect(detectPlatform("web")).toEqual({ os: "win32", arch: "x64" });
    const executable = resolveBinaryPath("web", {
      resolve: (specifier) => consumerRequire.resolve(specifier),
    });
    expect(executable).toBe(join(nativeRoot, "bin", "web.exe"));
    expect(executable.endsWith("\\bin\\web.exe")).toBe(true);

    const { stdout } = await execFileAsync(executable, ["--help"], { cwd: installRoot });
    expect(stdout).toContain("Search the web");
  });
});
