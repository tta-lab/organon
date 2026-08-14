import { isAbsolute, resolve as nodeResolve } from "node:path";

/**
 * Resolves a src tool path the way Pi's built-in tools resolve theirs: strip
 * an accidental leading @, then resolve absolute paths directly or relative to
 * the session working directory. src has no project-registry dependency.
 */
export function resolveSourcePath(rawPath: string, cwd: string): string {
  let normalized = rawPath;
  if (normalized.startsWith("@")) {
    normalized = normalized.slice(1);
  }
  return isAbsolute(normalized) ? nodeResolve(normalized) : nodeResolve(cwd, normalized);
}
