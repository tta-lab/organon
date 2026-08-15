// Vitest fixtures live in a disposable package-shaped workspace. Keeping them
// outside the repository prevents tests from creating, replacing, or deleting
// development Go binaries in packages/native/*/bin.
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const workspace = join(dirname(fileURLToPath(import.meta.url)));
const TOOLS = ["src", "web", "project", "og"] as const;

function hostSuffix(): string {
  const osName =
    process.platform === "darwin" ? "darwin" : process.platform === "linux" ? "linux" : "";
  const archName = process.arch === "arm64" ? "arm64" : process.arch === "x64" ? "x64" : "";
  if (
    (osName === "darwin" && archName !== "arm64") ||
    !osName ||
    !archName ||
    (osName === "linux" && !["arm64", "x64"].includes(archName))
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
      const sourcePackage = packageDirectory(workspace, tool, suffix);
      const destinationPackage = packageDirectory(testWorkspace, tool, suffix);
      const destinationBin = join(destinationPackage, "bin");
      mkdirSync(destinationBin, { recursive: true });
      copyFileSync(join(sourcePackage, "package.json"), join(destinationPackage, "package.json"));
      const destination = join(destinationBin, tool);
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
