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

/** Normalizes cobra "Error: ..." stderr lines into one concise message. */
export function cliError(stderr: string, exitCode: number): Error {
  const detail = stderr
    .trim()
    .replace(/^Error:\s*/m, "")
    .replace(/\n/g, " ");
  if (detail !== "") {
    return new Error(detail);
  }
  return new Error(`command exited with code ${exitCode}`);
}

/** Standard cancellation error matching pi's built-in tool behavior. */
export function abortError(): Error {
  return new Error("Operation aborted");
}
