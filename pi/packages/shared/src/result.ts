import { truncateForModel } from "./truncate.js";

const MAX_ERROR_LINES = 100;
const MAX_ERROR_BYTES = 8 * 1024;

/**
 * Parses exactly one JSON document from CLI stdout. Trailing newlines are
 * tolerated; multiple documents or non-JSON output are treated as a contract
 * violation and surface as a concise tool error.
 */
export function parseSingleJsonDoc<T>(stdout: string): T {
  const text = stdout.trim();
  if (text === "") {
    throw new Error("CLI produced no JSON output");
  }
  try {
    return JSON.parse(text) as T;
  } catch {
    const preview = text.length > 200 ? text.slice(0, 200) + "..." : text;
    throw new Error(`CLI produced invalid JSON output: ${preview}`);
  }
}

/**
 * Normalizes Cobra stderr into a concise model-facing error. Oversized stderr
 * is saved verbatim and replaced by a bounded preview with its full path.
 */
export async function cliError(stderr: string, exitCode: number): Promise<Error> {
  const detail = stderr
    .trim()
    .replace(/^Error:\s*/m, "")
    .replace(/\s+/g, " ");
  if (detail === "") {
    return new Error(`command exited with code ${exitCode}`);
  }
  const model = await truncateForModel(detail, {
    maxLines: MAX_ERROR_LINES,
    maxBytes: MAX_ERROR_BYTES,
    fullOutput: stderr,
    hint: "Inspect the saved stderr before retrying.",
  });
  return new Error(model.text);
}
