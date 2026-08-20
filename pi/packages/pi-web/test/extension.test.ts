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

async function expectDocsRejectedBeforeBinary(params: unknown, error: RegExp): Promise<void> {
  const directory = mkdtempSync(join(tmpdir(), "pi-web-invalid-"));
  const invocationPath = join(directory, "invocations");
  const priorInvocationPath = process.env.PI_WEB_TEST_INVOCATIONS;
  process.env.PI_WEB_TEST_INVOCATIONS = invocationPath;
  try {
    await expect(call("docs", params)).rejects.toThrow(error);
    expect(existsSync(invocationPath)).toBe(false);
  } finally {
    if (priorInvocationPath === undefined) delete process.env.PI_WEB_TEST_INVOCATIONS;
    else process.env.PI_WEB_TEST_INVOCATIONS = priorInvocationPath;
    rmSync(directory, { recursive: true, force: true });
  }
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
  const priorHome = process.env.HOME;
  const priorXDG = process.env.XDG_CACHE_HOME;
  const priorLocalAppData = process.env.LOCALAPPDATA;
  const home = mkdtempSync(join(tmpdir(), "pi-web-home-"));
  process.env.HOME = home;
  delete process.env.XDG_CACHE_HOME;
  delete process.env.LOCALAPPDATA;
  try {
    return await fn();
  } finally {
    if (priorHome === undefined) delete process.env.HOME;
    else process.env.HOME = priorHome;
    if (priorXDG === undefined) delete process.env.XDG_CACHE_HOME;
    else process.env.XDG_CACHE_HOME = priorXDG;
    if (priorLocalAppData === undefined) delete process.env.LOCALAPPDATA;
    else process.env.LOCALAPPDATA = priorLocalAppData;
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
  });

  it("exposes direct object schemas and keeps grouped docs action fields runtime-strict", async () => {
    expect((webSearchSchema as any).type).toBe("object");
    expect((webDocsSchema as any).type).toBe("object");
    expect((webSgraphSchema as any).type).toBe("object");
    expect(Value.Check(webSearchSchema, { queries: ["x"] })).toBe(true);
    expect(Value.Check(webSearchSchema, { queries: [] })).toBe(false);
    expect(Value.Check(webSearchSchema, { queries: ["x", "y", "z", "w", "v"] })).toBe(false);
    expect(Value.Check(webSearchSchema, { queries: [" "] })).toBe(false);
    expect(Value.Check(webSearchSchema, { query: "x" })).toBe(false);
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

  it("validates batched search before launching the binary", async () => {
    const directory = mkdtempSync(join(tmpdir(), "pi-web-search-invalid-"));
    const invocationPath = join(directory, "invocations");
    const priorInvocationPath = process.env.PI_WEB_TEST_INVOCATIONS;
    process.env.PI_WEB_TEST_INVOCATIONS = invocationPath;
    try {
      await expect(call("search", { queries: [] })).rejects.toThrow(/1 to 4 non-empty strings/);
      await expect(call("search", { queries: ["", "valid"] })).rejects.toThrow(
        /1 to 4 non-empty strings/,
      );
      await expect(call("search", { queries: [" "] })).rejects.toThrow(/1 to 4 non-empty strings/);
      await expect(call("search", { queries: ["x", "y", "z", "w", "v"] })).rejects.toThrow(
        /1 to 4 non-empty strings/,
      );
      await expect(call("search", { query: "legacy" })).rejects.toThrow(
        /does not accept field query/,
      );
      expect(existsSync(invocationPath)).toBe(false);
    } finally {
      if (priorInvocationPath === undefined) delete process.env.PI_WEB_TEST_INVOCATIONS;
      else process.env.PI_WEB_TEST_INVOCATIONS = priorInvocationPath;
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("deduplicates exact queries, runs them concurrently, and merges round-robin", async () => {
    const directory = mkdtempSync(join(tmpdir(), "pi-web-search-batch-"));
    const invocationPath = join(directory, "invocations");
    const eventsPath = join(directory, "events");
    const priorInvocationPath = process.env.PI_WEB_TEST_INVOCATIONS;
    const priorEventsPath = process.env.PI_WEB_TEST_SEARCH_EVENTS;
    process.env.PI_WEB_TEST_INVOCATIONS = invocationPath;
    process.env.PI_WEB_TEST_SEARCH_EVENTS = eventsPath;
    try {
      const result = await call("search", { queries: ["slow", "fast", "slow"] });
      const details = result.details as {
        provider: string;
        results: Array<{ title: string; position: number }>;
      };
      expect(details.provider).toBe("DuckDuckGo");
      expect(details.results.map((item) => item.title)).toEqual([
        "Result 1 for slow",
        "Result 1 for fast",
        "Result 2 for slow",
        "Result 2 for fast",
      ]);
      expect(details.results.map((item) => item.position)).toEqual([1, 2, 3, 4]);

      const invocations = readFileSync(invocationPath, "utf8")
        .trim()
        .split("\n")
        .map((line) => JSON.parse(line) as string[]);
      expect(invocations).toHaveLength(2);
      expect(invocations.map((args) => args[1])).toEqual(expect.arrayContaining(["slow", "fast"]));

      const events = readFileSync(eventsPath, "utf8")
        .trim()
        .split("\n")
        .map((line) => JSON.parse(line) as { phase: string; query: string });
      const firstEnd = events.findIndex((event) => event.phase === "end");
      expect(events.slice(0, firstEnd).filter((event) => event.phase === "start")).toHaveLength(2);
      expect(events.filter((event) => event.phase === "end")).toHaveLength(2);
    } finally {
      if (priorInvocationPath === undefined) delete process.env.PI_WEB_TEST_INVOCATIONS;
      else process.env.PI_WEB_TEST_INVOCATIONS = priorInvocationPath;
      if (priorEventsPath === undefined) delete process.env.PI_WEB_TEST_SEARCH_EVENTS;
      else process.env.PI_WEB_TEST_SEARCH_EVENTS = priorEventsPath;
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("waits for every started search before reporting a batch failure", async () => {
    const directory = mkdtempSync(join(tmpdir(), "pi-web-search-failure-"));
    const eventsPath = join(directory, "events");
    const priorEventsPath = process.env.PI_WEB_TEST_SEARCH_EVENTS;
    process.env.PI_WEB_TEST_SEARCH_EVENTS = eventsPath;
    try {
      await expect(call("search", { queries: ["boom", "slow"] })).rejects.toThrow(
        /search backend failed/,
      );
      const events = readFileSync(eventsPath, "utf8")
        .trim()
        .split("\n")
        .map((line) => JSON.parse(line) as { phase: string; query: string });
      expect(events.filter((event) => event.phase === "start").map((event) => event.query)).toEqual(
        expect.arrayContaining(["boom", "slow"]),
      );
      expect(events.filter((event) => event.phase === "end").map((event) => event.query)).toEqual(
        expect.arrayContaining(["boom", "slow"]),
      );
    } finally {
      if (priorEventsPath === undefined) delete process.env.PI_WEB_TEST_SEARCH_EVENTS;
      else process.env.PI_WEB_TEST_SEARCH_EVENTS = priorEventsPath;
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("searches, fetches natively, resolves and fetches docs, and searches source", async () => {
    const search = await call("search", { queries: ["tree-sitter"] });
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
      const resolved = await call("docs", { action: "resolve", query: "dispatch-proof" });
      expect((resolved.details as { libraries: Array<{ id: string }> }).libraries[0]!.id).toBe(
        "/dispatch-proof",
      );
      const fetchedDocs = await call("docs", {
        action: "fetch",
        library_id: "/dispatch-proof",
        topic: "hooks",
        tokens: 500,
      });
      expect((fetchedDocs.details as { content: string }).content).toContain("docs content");
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

  it("rejects missing, irrelevant, malformed, and cross-action docs fields before the CLI", async () => {
    await expectDocsRejectedBeforeBinary({ action: "resolve" }, /query must be a non-empty string/);
    await expectDocsRejectedBeforeBinary(
      { action: "fetch" },
      /library_id must be a non-empty string/,
    );
    await expectDocsRejectedBeforeBinary(
      { action: "resolve", query: "x", library_id: "/wrong" },
      /does not accept library_id/,
    );
    await expectDocsRejectedBeforeBinary(
      { action: "fetch", library_id: "/x", query: "wrong" },
      /does not accept query/,
    );
    await expectDocsRejectedBeforeBinary(
      { action: "resolve", query: 42 },
      /query must be a non-empty string/,
    );
    await expectDocsRejectedBeforeBinary(
      { action: "fetch", library_id: "/x", topic: 42 },
      /topic must be a string/,
    );
    await expectDocsRejectedBeforeBinary(
      { action: "fetch", library_id: "/x", tokens: 1.5 },
      /tokens must be an integer/,
    );
    await expectDocsRejectedBeforeBinary(
      { action: "unknown", query: "x" },
      /action must be "resolve" or "fetch"/,
    );
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
    await expect(
      call("search", { queries: ["wait-for-abort"] }, controller.signal),
    ).rejects.toThrow("Operation aborted");
    expect(existsSync("/nonexistent/pi-web-test")).toBe(false);
  });
});
