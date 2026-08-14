import { readFileSync, rmSync } from "node:fs";
import { dirname } from "node:path";

import { describe, expect, it } from "vitest";

import { truncateForModel } from "../src/truncate.js";

describe("truncateForModel", () => {
  it("saves complete line-truncated output and tells the model where it is", async () => {
    const content = Array.from({ length: 3000 }, (_, index) => `line ${index}`).join("\n");
    const result = await truncateForModel(content, { hint: "Use a narrower query." });

    try {
      expect(result.truncation?.truncated).toBe(true);
      expect(result.fullOutputPath).toBeTruthy();
      expect(readFileSync(result.fullOutputPath!, "utf8")).toBe(content);
      expect(result.text).toContain("Use a narrower query.");
      expect(result.text).toContain(`Full output saved to: ${result.fullOutputPath}`);
    } finally {
      if (result.fullOutputPath) {
        rmSync(dirname(result.fullOutputPath), { recursive: true, force: true });
      }
    }
  });

  it("keeps action-specific guidance for an oversized first line", async () => {
    const result = await truncateForModel("x".repeat(60 * 1024), {
      hint: "Use src read with a narrower range.",
    });

    try {
      expect(result.truncation?.firstLineExceedsLimit).toBe(true);
      expect(result.text).toContain("Use src read with a narrower range.");
      expect(result.text).toContain(`Full output saved to: ${result.fullOutputPath}`);
    } finally {
      if (result.fullOutputPath) {
        rmSync(dirname(result.fullOutputPath), { recursive: true, force: true });
      }
    }
  });
});
