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
