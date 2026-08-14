// Stages the four test fixture binaries into the workspace native packages
// ONCE per vitest process, before any test file runs, so concurrent test files
// never race on the shared fixture path. The returned teardown removes them
// when the run finishes, keeping the workspace native packages clean for local
// debugging (a leftover fixture would otherwise be picked up as the tool
// binary).
import { chmodSync, copyFileSync, mkdirSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const workspace = join(dirname(fileURLToPath(import.meta.url)));
const osName = process.platform === "darwin" ? "darwin" : "linux";
const archName = process.arch === "arm64" ? "arm64" : "x64";
const TOOLS = ["src", "web", "project", "og"] as const;

function fixturePath(tool: string): string {
  return join(workspace, "packages", "native", `pi-${tool}-${osName}-${archName}`, "bin", tool);
}

export default function setup(): () => void {
  for (const tool of TOOLS) {
    const source = join(workspace, "packages", `pi-${tool}`, "testdata", "bin", tool);
    const destDir = join(
      workspace,
      "packages",
      "native",
      `pi-${tool}-${osName}-${archName}`,
      "bin",
    );
    mkdirSync(destDir, { recursive: true });
    const dest = join(destDir, tool);
    copyFileSync(source, dest);
    chmodSync(dest, 0o755);
  }
  return () => {
    for (const tool of TOOLS) {
      rmSync(fixturePath(tool), { force: true });
    }
  };
}
