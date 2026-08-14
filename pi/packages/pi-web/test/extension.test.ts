import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import { truncateForModel, webSchema, webTool } from "../src/tool.js";

const def = webTool();
const ctx = { cwd: "/tmp", model: undefined } as any;

function call(params: unknown) {
  return def.execute("call-1", params as any, undefined, undefined, ctx);
}

describe("pi-web extension", () => {
  it("registers one closed-union web tool with guidelines naming web", async () => {
    const { registerWebTool } = await import("../src/tool.js");
    const registered: any[] = [];
    registerWebTool({ registerTool: (d: any) => registered.push(d) } as any);
    expect(registered).toHaveLength(1);
    expect(registered[0]!.name).toBe("web");
  });

  it("validates the closed action union with action-specific required fields", () => {
    expect(Value.Check(webSchema, { action: "search", query: "x" })).toBe(true);
    expect(Value.Check(webSchema, { action: "fetch", url: "https://example.com" })).toBe(true);
    expect(
      Value.Check(webSchema, {
        action: "fetch",
        url: "https://example.com",
        tree: true,
        section_id: "a1",
        tree_threshold: 100,
      }),
    ).toBe(true);
    expect(Value.Check(webSchema, { action: "docs_resolve", query: "x" })).toBe(true);
    expect(Value.Check(webSchema, { action: "docs_fetch", library_id: "/x" })).toBe(true);
    expect(Value.Check(webSchema, { action: "sgraph", query: "repo:x" })).toBe(true);
    expect(Value.Check(webSchema, { action: "search" })).toBe(false);
    expect(Value.Check(webSchema, { action: "fetch", url: "https://example.com", bogus: 1 })).toBe(
      false,
    );
    expect(Value.Check(webSchema, { action: "nope", query: "x" })).toBe(false);
  });

  it("search action passes the query and renders provider results", async () => {
    const result = await call({ action: "search", query: "tree-sitter" });
    expect((result.content[0] as { text: string }).text).toContain("DuckDuckGo");
    expect((result.content[0] as { text: string }).text).toContain("tree-sitter");
    const details = result.details as { provider: string; results: unknown[] };
    expect(details.provider).toBe("DuckDuckGo");
    expect(details.results).toHaveLength(1);
  });

  it("search backend failure surfaces as a concise error", async () => {
    await expect(call({ action: "search", query: "boom" })).rejects.toThrow(/rate limited/);
  });

  it("fetch action maps tree/section/full/tree-threshold flags and returns structured details", async () => {
    const result = await call({
      action: "fetch",
      url: "https://example.com/page",
      tree: true,
      section_id: "b2",
      tree_threshold: 999,
    });
    const details = result.details as {
      url: string;
      mode: string;
      content: string;
      truncation?: unknown;
    };
    expect(details.url).toBe("https://example.com/page");
    expect(details.mode).toBe("tree");
    expect(details.content).toContain("threshold 999");
    expect((details as any).truncation).toBeUndefined();
    expect((result.content[0] as { text: string }).text).toContain("rendered content");
  });

  it("fetch failure surfaces as a concise error", async () => {
    await expect(call({ action: "fetch", url: "https://boom.example" })).rejects.toThrow(
      /connection refused/,
    );
  });

  it("docs_resolve passes the query and returns library records", async () => {
    const result = await call({ action: "docs_resolve", query: "react" });
    const details = result.details as { query: string; libraries: Array<{ id: string }> };
    expect(details.query).toBe("react");
    expect(details.libraries[0]!.id).toBe("/react");
  });

  it("docs_fetch passes topic and tokens and truncates large content", async () => {
    const result = await call({
      action: "docs_fetch",
      library_id: "/react",
      topic: "hooks",
      tokens: 500,
    });
    const details = result.details as { library_id: string; topic: string; content: string };
    expect(details.library_id).toBe("/react");
    expect(details.topic).toBe("hooks");
    expect(details.content).toContain("docs content");
  });

  it("sgraph action maps count/context/timeout flags", async () => {
    const result = await call({
      action: "sgraph",
      query: "repo:tta-lab/organon srcop",
      count: 14,
      context: 3,
      timeout: 9,
    });
    expect((result.content[0] as { text: string }).text).toContain("matches");
  });

  it("large text results are truncated with an actionable continuation notice", () => {
    const big = Array.from({ length: 3000 }, (_, i) => "line " + i).join("\n");
    const { text, truncation } = truncateForModel(big);
    expect(truncation!.truncated).toBe(true);
    expect(truncation!.truncatedBy).toBe("lines");
    expect(text).toContain("[Truncated: showing 2000 of 3000 lines");
    expect(text).toContain("section_id");
  });

  it("abort cancels the CLI child process", async () => {
    const controller = new AbortController();
    controller.abort();
    await expect(
      def.execute("call-2", { action: "search", query: "x" }, controller.signal, undefined, ctx),
    ).rejects.toThrow("Operation aborted");
  });
});
