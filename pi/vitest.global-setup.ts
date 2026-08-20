// Vitest fixtures live in a disposable package-shaped workspace. Keeping them
// outside the repository prevents tests from creating, replacing, or deleting
// development Go binaries in packages/native/*/bin.
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { NATIVE_TOOLS } from "./scripts/release-targets.mjs";

const workspace = join(dirname(fileURLToPath(import.meta.url)));
const TOOLS = NATIVE_TOOLS;

function hostSuffix(): string {
  const osName =
    process.platform === "darwin"
      ? "darwin"
      : process.platform === "linux"
        ? "linux"
        : process.platform === "win32"
          ? "win32"
          : "";
  const archName = process.arch === "arm64" ? "arm64" : process.arch === "x64" ? "x64" : "";
  if (
    (osName === "darwin" && archName !== "arm64") ||
    !osName ||
    !archName ||
    (osName === "linux" && !["arm64", "x64"].includes(archName)) ||
    (osName === "win32" && archName !== "x64")
  ) {
    throw new Error(
      `Vitest fixture workspace does not support ${process.platform}-${process.arch}`,
    );
  }
  return `${osName}-${archName}`;
}

function packageDirectory(root: string, tool: string, suffix: string): string {
  return join(root, "packages", "native", `pi-${tool}-${suffix}`);
}

export default function setup(): () => void {
  const suffix = hostSuffix();
  const testWorkspace = mkdtempSync(join(tmpdir(), "organon-pi-test-workspace-"));
  const previousWorkspace = process.env.ORGANON_PI_TEST_WORKSPACE;
  try {
    for (const tool of TOOLS) {
      if (suffix === "win32-x64" && tool !== "web") continue;
      const sourcePackage = packageDirectory(workspace, tool, suffix);
      const destinationPackage = packageDirectory(testWorkspace, tool, suffix);
      const destinationBin = join(destinationPackage, "bin");
      mkdirSync(destinationBin, { recursive: true });
      copyFileSync(join(sourcePackage, "package.json"), join(destinationPackage, "package.json"));
      const executable = suffix === "win32-x64" ? `${tool}.exe` : tool;
      const destination = join(destinationBin, executable);
      copyFileSync(join(workspace, "packages", `pi-${tool}`, "testdata", "bin", tool), destination);
      chmodSync(destination, 0o755);
    }
    process.env.ORGANON_PI_TEST_WORKSPACE = testWorkspace;
  } catch (error) {
    if (previousWorkspace === undefined) delete process.env.ORGANON_PI_TEST_WORKSPACE;
    else process.env.ORGANON_PI_TEST_WORKSPACE = previousWorkspace;
    rmSync(testWorkspace, { recursive: true, force: true });
    throw error;
  }

  return () => {
    if (previousWorkspace === undefined) delete process.env.ORGANON_PI_TEST_WORKSPACE;
    else process.env.ORGANON_PI_TEST_WORKSPACE = previousWorkspace;
    rmSync(testWorkspace, { recursive: true, force: true });
  };
}
