import { homedir } from "node:os";
import { isAbsolute, join, resolve as nodeResolve } from "node:path";
import { fileURLToPath } from "node:url";

/**
 * Resolves a src tool path the way Pi's built-in tools resolve theirs:
 * trim, normalize unicode spaces, strip an accidental leading @, expand ~,
 * convert file:// URLs, then resolve absolute paths directly or relative to
 * the session working directory. src has no project-registry dependency.
 */
export function resolveSourcePath(rawPath: string, cwd: string): string {
  let normalized = rawPath.trim();
  normalized = normalized.replace(/[\u00A0\u2000-\u200A\u202F\u205F\u3000]/g, " ");
  if (normalized.startsWith("@")) {
    normalized = normalized.slice(1);
  }
  if (normalized === "~") {
    return homedir();
  }
  if (normalized.startsWith("~/")) {
    return join(homedir(), normalized.slice(2));
  }
  if (normalized.startsWith("file://")) {
    return fileURLToPath(normalized);
  }
  return isAbsolute(normalized) ? nodeResolve(normalized) : nodeResolve(cwd, normalized);
}
