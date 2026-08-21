import { createRequire } from "node:module";
import { dirname, join } from "node:path";

import { detectPlatform } from "./platform.js";

/** Resolves an npm specifier to a path; injectable for tests. */
export type BinaryResolver = (specifier: string) => string;

/** Name of the native npm package that carries the binary for `tool`. */
export function nativePackageName(tool: string): string {
  const { os, arch } = detectPlatform(tool);
  return `@tta-lab/pi-${tool}-${os}-${arch}`;
}

export interface BinaryResolutionOptions {
  /** Module-bound require used to resolve the native package; defaults to the shared module's own require. Pass createRequire(import.meta.url) from the extension entry so resolution stays correct when the shared code is bundled into the extension. */
  require?: NodeRequire;
  /** Injectable specifier resolver for tests. */
  resolve?: BinaryResolver;
}

/**
 * Resolves the package-local Go binary for `tool`. Never consults PATH,
 * GitHub releases, or the network: the binary must be installed as the
 * matching optional native package. Unsupported platforms throw an
 * actionable error from detectPlatform.
 */
export function resolveBinaryPath(tool: string, options?: BinaryResolutionOptions): string {
  const { os, arch } = detectPlatform(tool);
  const pkg = `@tta-lab/pi-${tool}-${os}-${arch}`;
  const executable = os === "win32" ? `${tool}.exe` : tool;
  const testWorkspace =
    process.env.VITEST === "true" ? process.env.ORGANON_PI_TEST_WORKSPACE : undefined;
  if (testWorkspace && !options?.resolve) {
    return join(testWorkspace, "packages", "native", `pi-${tool}-${os}-${arch}`, "bin", executable);
  }
  const require = options?.require ?? createRequire(import.meta.url);
  const resolver = options?.resolve ?? ((specifier: string) => require.resolve(specifier));
  let packageRoot: string;
  try {
    packageRoot = dirname(resolver(`${pkg}/package.json`));
  } catch {
    throw new Error(
      `native package ${pkg} is not installed for this platform; reinstall the extension ` +
        `or install ${pkg} at the exact same version so the matching binary is available.`,
    );
  }
  return join(packageRoot, "bin", executable);
}
