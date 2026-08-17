import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";
import {
  webDocsSchema,
  webDocsTool,
  webFetchTool,
  webSearchSchema,
  webSearchTool,
  webSgraphSchema,
  webSgraphTool,
  registerWebTools,
} from "../src/tool.js";

const definitions = {
  search: webSearchTool(),
  fetch: webFetchTool(),
  docs: webDocsTool(),
  sgraph: webSgraphTool(),
} as const;
type ToolName = keyof typeof definitions;
const ctx = { cwd: "/tmp", model: undefined } as any;

function call(tool: ToolName, params: unknown, signal?: AbortSignal) {
  return definitions[tool].execute("call-1", params as any, signal, undefined, ctx);
}

async function startServer(handler: (req: IncomingMessage, res: ServerResponse) => void): Promise<{
  server: Server;
  url: string;
}> {
  const server = createServer(handler);
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("server did not bind");
  return { server, url: `http://127.0.0.1:${address.port}` };
}

async function withTempHome<T>(fn: () => Promise<T>): Promise<T> {
  const prior = process.env.HOME;
  const home = mkdtempSync(join(tmpdir(), "pi-web-home-"));
  process.env.HOME = home;
  try {
    return await fn();
  } finally {
    if (prior === undefined) delete process.env.HOME;
    else process.env.HOME = prior;
    rmSync(home, { recursive: true, force: true });
  }
}

describe("pi-web extension", () => {
  it("registers the four capability tools and removes the old mega-tool", () => {
    const registered: any[] = [];
    registerWebTools({ registerTool: (definition: any) => registered.push(definition) });
    expect(registered.map((definition) => definition.name)).toEqual([
      "web_search",
      "web_fetch",
      "web_docs",
      "web_sgraph",
    ]);
    expect(registered.every((definition) => definition.name !== "web")).toBe(true);
  });

  it("exposes direct object schemas and keeps grouped docs action fields runtime-strict", async () => {
    expect((webSearchSchema as any).type).toBe("object");
    expect((webDocsSchema as any).type).toBe("object");
    expect((webSgraphSchema as any).type).toBe("object");
    expect(Value.Check(webSearchSchema, { query: "x" })).toBe(true);
    expect(Value.Check(webSearchSchema, {})).toBe(false);
    expect(Value.Check(webDocsSchema, { action: "resolve", query: "x" })).toBe(true);
    expect(Value.Check(webDocsSchema, { action: "fetch", library_id: "/x" })).toBe(true);
    expect(Value.Check(webDocsSchema, { action: "fetch", library_id: "/x", tokens: 1.5 })).toBe(
      false,
    );
    expect(Value.Check(webDocsSchema, { action: "nope", query: "x" })).toBe(false);
    await expect(
      call("docs", { action: "resolve", query: "x", library_id: "/wrong" }),
    ).rejects.toThrow(/does not accept library_id/);
    await expect(
      call("docs", { action: "fetch", library_id: "/x", query: "wrong" }),
    ).rejects.toThrow(/does not accept query/);
  });

  it("searches, fetches natively, resolves and fetches docs, and searches source", async () => {
    const search = await call("search", { query: "tree-sitter" });
    expect((search.content[0] as { text: string }).text).toContain("tree-sitter");

    const { server, url } = await startServer((_req, res) => {
      res.setHeader("Content-Type", "text/html; charset=utf-8");
      res.end("<article><h1>Test page</h1><h2>Install</h2><p>Install content.</p></article>");
    });
    const directory = mkdtempSync(join(tmpdir(), "pi-web-invocations-"));
    const invocationPath = join(directory, "invocations");
    const priorInvocationPath = process.env.PI_WEB_TEST_INVOCATIONS;
    process.env.PI_WEB_TEST_INVOCATIONS = invocationPath;
    try {
      const fetched = await withTempHome(() => call("fetch", { url, tree: true }));
      expect((fetched.details as { mode: string }).mode).toBe("tree");
      await call("docs", { action: "resolve", query: "dispatch-proof" });
      await call("docs", {
        action: "fetch",
        library_id: "/dispatch-proof",
        topic: "hooks",
        tokens: 500,
      });
      await call("sgraph", { query: "dispatch-proof", count: 14, context: 3, timeout: 9 });
      const invocations = readFileSync(invocationPath, "utf8");
      expect(invocations).toContain('["docs","resolve","dispatch-proof","--json"]');
      expect(invocations).toContain(
        '["docs","fetch","/dispatch-proof","hooks","--tokens","500","--json"]',
      );
      expect(invocations).toContain(
        '["sgraph","dispatch-proof","--json","--count","14","--context","3","--timeout","9"]',
      );
      expect(invocations).not.toContain('["fetch"');
    } finally {
      if (priorInvocationPath === undefined) delete process.env.PI_WEB_TEST_INVOCATIONS;
      else process.env.PI_WEB_TEST_INVOCATIONS = priorInvocationPath;
      rmSync(directory, { recursive: true, force: true });
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("preserves bounded fetch output and abort behavior", async () => {
    const { server, url } = await startServer((_req, res) => {
      res.setHeader("Content-Type", "text/plain");
      res.end(Array.from({ length: 3000 }, (_, index) => `line ${index}`).join("\n"));
    });
    try {
      const result = await withTempHome(() => call("fetch", { url }));
      const details = result.details as { fullOutputPath?: string; content: string };
      expect(details.fullOutputPath).toBeTruthy();
      expect((result.content[0] as { text: string }).text).toContain("section_id");
      expect(readFileSync(details.fullOutputPath!, "utf8")).toBe(details.content);
      rmSync(dirname(details.fullOutputPath!), { recursive: true, force: true });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }

    const controller = new AbortController();
    controller.abort();
    await expect(call("search", { query: "wait-for-abort" }, controller.signal)).rejects.toThrow(
      "Operation aborted",
    );
    expect(existsSync("/nonexistent/pi-web-test")).toBe(false);
  });
});
