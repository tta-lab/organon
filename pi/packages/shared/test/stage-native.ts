import { chmodSync, copyFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/**
 * Stages a test fake binary into the host-platform native package so the
 * shared binary resolver resolves it exactly as a published install would.
 * The native bin/ directories are generated artifacts (gitignored); real
 * release binaries replace these fakes.
 */
export function stageNativeBinary(tool: string, fakeSource: string): string {
  const here = dirname(fileURLToPath(import.meta.url));
  const workspaceRoot = join(here, "..", "..", "..");
  const { platform, arch } = process;
  const osName = platform === "darwin" ? "darwin" : "linux";
  const archName = arch === "arm64" ? "arm64" : "x64";
  const destDir = join(
    workspaceRoot,
    "packages",
    "native",
    `pi-${tool}-${osName}-${archName}`,
    "bin",
  );
  const dest = join(destDir, tool);
  mkdirSync(destDir, { recursive: true });
  copyFileSync(fakeSource, dest);
  chmodSync(dest, 0o755);
  return dest;
}
