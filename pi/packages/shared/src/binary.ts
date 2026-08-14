import { createRequire } from "node:module";
import { dirname, join } from "node:path";

import { detectPlatform, type PlatformTriple } from "./platform.js";

/** Resolves an npm specifier to a path; injectable for tests. */
export type BinaryResolver = (specifier: string) => string;

/** Name of the native npm package that carries the binary for `tool`. */
export function nativePackageName(tool: string): string {
  const { os, arch } = detectPlatform();
  return `@tta-lab/pi-${tool}-${os}-${arch}`;
}

/**
 * Resolves the package-local Go binary for `tool`. Never consults PATH,
 * GitHub releases, or the network: the binary must be installed as the
 * matching optional native package. Unsupported platforms throw an
 * actionable error from detectPlatform.
 */
export function resolveBinaryPath(tool: string, resolve?: BinaryResolver): string {
  const pkg = nativePackageName(tool);
  const require = createRequire(import.meta.url);
  const resolver = resolve ?? ((specifier: string) => require.resolve(specifier));
  let packageRoot: string;
  try {
    packageRoot = dirname(resolver(`${pkg}/package.json`));
  } catch {
    throw new Error(
      `native package ${pkg} is not installed for this platform; reinstall the extension ` +
        `or install ${pkg} at the exact same version so the matching binary is available.`,
    );
  }
  return join(packageRoot, "bin", tool);
}
