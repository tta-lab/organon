import { readFileSync, rmSync } from "node:fs";
import { dirname } from "node:path";

import { describe, expect, it } from "vitest";

import { modelTextResult } from "../src/index.js";

function cleanup(result: { details: unknown }): void {
  const path = (result.details as { fullOutputPath?: string }).fullOutputPath;
  if (path) {
    rmSync(dirname(path), { recursive: true, force: true });
  }
}

describe("modelTextResult", () => {
  it("returns ordinary action output as one text block with unmodified details", async () => {
    const data = { provider: "test", results: ["one"] };
    const result = await modelTextResult(data, "one result");

    expect(result).toEqual({
      content: [{ type: "text", text: "one result" }],
      details: data,
    });
  });

  it("bounds line-heavy output and retains the complete action result", async () => {
    const content = Array.from({ length: 3000 }, (_, index) => `line ${index}`).join("\n");
    const result = await modelTextResult({ action: "search" }, content, {
      hint: "Use a narrower query.",
    });
    const details = result.details as {
      truncation: { truncated: boolean; truncatedBy: string };
      fullOutputPath: string;
    };

    try {
      expect(details.truncation).toMatchObject({ truncated: true, truncatedBy: "lines" });
      expect(result.content[0]!.text).toContain("Use a narrower query.");
      expect(result.content[0]!.text).toContain(`Full output saved to: ${details.fullOutputPath}`);
      expect(readFileSync(details.fullOutputPath, "utf8")).toBe(content);
    } finally {
      cleanup(result);
    }
  });

  it("bounds byte-heavy output without splitting a line", async () => {
    const content = Array.from({ length: 20 }, () => "x".repeat(24)).join("\n");
    const result = await modelTextResult({ action: "fetch" }, content, {
      maxLines: 100,
      maxBytes: 100,
      hint: "Use a smaller section.",
    });
    const details = result.details as {
      truncation: { truncatedBy: string; outputBytes: number };
      fullOutputPath: string;
    };

    try {
      expect(details.truncation.truncatedBy).toBe("bytes");
      expect(details.truncation.outputBytes).toBeLessThanOrEqual(100);
      expect(result.content[0]!.text).toContain("Use a smaller section.");
      expect(readFileSync(details.fullOutputPath, "utf8")).toBe(content);
    } finally {
      cleanup(result);
    }
  });

  it("keeps the action hint and complete path when the first line exceeds the byte cap", async () => {
    const content = "x".repeat(200);
    const result = await modelTextResult({ action: "read" }, content, {
      maxBytes: 100,
      hint: "Use a narrower read range.",
    });
    const details = result.details as {
      truncation: { firstLineExceedsLimit: boolean };
      fullOutputPath: string;
    };

    try {
      expect(details.truncation.firstLineExceedsLimit).toBe(true);
      expect(result.content[0]!.text).toContain("Use a narrower read range.");
      expect(result.content[0]!.text).toContain(`Full output saved to: ${details.fullOutputPath}`);
      expect(readFileSync(details.fullOutputPath, "utf8")).toBe(content);
    } finally {
      cleanup(result);
    }
  });
});
