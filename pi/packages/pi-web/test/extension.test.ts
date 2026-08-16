import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";
import { webSchema, webTool } from "../src/tool.js";

const def = webTool();
const ctx = { cwd: "/tmp", model: undefined } as any;

function call(params: unknown, signal?: AbortSignal) {
  return def.execute("call-1", params as any, signal, undefined, ctx);
}

async function waitForFile(path: string): Promise<void> {
  const deadline = Date.now() + 1000;
  while (Date.now() < deadline) {
    if (existsSync(path)) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error(`child did not write ${path}`);
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
    expect(
      Value.Check(webSchema, { action: "fetch", url: "https://example.com", tree_threshold: 1.5 }),
    ).toBe(false);
    expect(Value.Check(webSchema, { action: "docs_fetch", library_id: "/x", tokens: 1.5 })).toBe(
      false,
    );
    expect(Value.Check(webSchema, { action: "sgraph", query: "repo:x", count: 2.5 })).toBe(false);
    // Negative integers match the MCP contract: the shared Go service
    // normalizes non-positive count/context and treats non-positive timeout
    // as disabled.
    expect(Value.Check(webSchema, { action: "sgraph", query: "repo:x", count: -1 })).toBe(true);
    expect(Value.Check(webSchema, { action: "sgraph", query: "repo:x", context: -1 })).toBe(true);
    expect(Value.Check(webSchema, { action: "sgraph", query: "repo:x", timeout: -1 })).toBe(true);
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

  it("fetches HTML in-process and preserves tree navigation details", async () => {
    const { server, url } = await startServer((_req, res) => {
      res.setHeader("Content-Type", "text/html; charset=utf-8");
      res.end(
        "<html><head><title>Test</title></head><body><article>" +
          "<h1>Test page</h1><h2>Install</h2><p>Install content.</p>" +
          "<h3>Details</h3><p>Details content.</p></article></body></html>",
      );
    });
    try {
      const result = await withTempHome(() =>
        call({ action: "fetch", url, tree: true, tree_threshold: 999 }),
      );
      const details = result.details as { url: string; mode: string; content: string };
      expect(details.url).toBe(url);
      expect(details.mode).toBe("tree");
      expect(details.content).toContain("Install");
      expect(details.content).toContain("Details");
      expect((result.content[0] as { text: string }).text).toContain("Use -s <id>");
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("fetches non-HTML text and rejects HTTP errors", async () => {
    const { server, url } = await startServer((req, res) => {
      if (req.url === "/error") {
        res.statusCode = 500;
        res.end("server error");
        return;
      }
      res.setHeader("Content-Type", "text/plain; charset=utf-8");
      res.end("plain text content");
    });
    try {
      await withTempHome(async () => {
        const result = await call({ action: "fetch", url });
        expect((result.details as { content: string }).content).toBe("plain text content");
        await expect(call({ action: "fetch", url: `${url}/error` })).rejects.toThrow("HTTP 500");
      });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("dispatches fetch natively while all other actions invoke the packaged binary", async () => {
    const { server, url } = await startServer((_req, res) => {
      res.setHeader("Content-Type", "text/plain");
      res.end("native fetch");
    });
    const directory = mkdtempSync(join(tmpdir(), "pi-web-invocations-"));
    const invocationPath = join(directory, "invocations");
    const priorInvocationPath = process.env.PI_WEB_TEST_INVOCATIONS;
    process.env.PI_WEB_TEST_INVOCATIONS = invocationPath;
    try {
      await withTempHome(async () => {
        await call({ action: "fetch", url });
        expect(existsSync(invocationPath)).toBe(false);

        await call({ action: "search", query: "dispatch-proof" });
        await call({ action: "docs_resolve", query: "dispatch-proof" });
        await call({ action: "docs_fetch", library_id: "/dispatch-proof" });
        await call({ action: "sgraph", query: "dispatch-proof" });
      });
      const invocations = readFileSync(invocationPath, "utf8");
      expect(invocations).toContain('["search","dispatch-proof","--json"]');
      expect(invocations).toContain('["docs","resolve","dispatch-proof","--json"]');
      expect(invocations).toContain('["docs","fetch","/dispatch-proof","--json"]');
      expect(invocations).toContain('["sgraph","dispatch-proof","--json"]');
    } finally {
      if (priorInvocationPath === undefined) delete process.env.PI_WEB_TEST_INVOCATIONS;
      else process.env.PI_WEB_TEST_INVOCATIONS = priorInvocationPath;
      rmSync(directory, { recursive: true, force: true });
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
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

  it("returns bounded model text with its complete structured content", async () => {
    const { server, url } = await startServer((_req, res) => {
      res.setHeader("Content-Type", "text/plain");
      res.end(Array.from({ length: 3000 }, (_, index) => `line ${index}`).join("\n"));
    });
    try {
      const result = await withTempHome(() => call({ action: "fetch", url }));
      const details = result.details as {
        content: string;
        truncation: { truncated: boolean; truncatedBy: string };
        fullOutputPath: string;
      };

      try {
        expect(details.truncation).toMatchObject({ truncated: true, truncatedBy: "lines" });
        expect((result.content[0] as { text: string }).text).toContain("section_id");
        expect((result.content[0] as { text: string }).text).toContain(
          `Full output saved to: ${details.fullOutputPath}`,
        );
        expect(readFileSync(details.fullOutputPath, "utf8")).toBe(details.content);
      } finally {
        if (details.fullOutputPath) {
          rmSync(dirname(details.fullOutputPath), { recursive: true, force: true });
        }
      }
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("forwards an abort that fires after the web child starts", async () => {
    const directory = mkdtempSync(join(tmpdir(), "pi-web-abort-"));
    const pidPath = join(directory, "pid");
    const priorPIDPath = process.env.PI_WEB_TEST_PID_FILE;
    process.env.PI_WEB_TEST_PID_FILE = pidPath;
    const controller = new AbortController();
    const pending = call({ action: "search", query: "wait-for-abort" }, controller.signal);

    try {
      await waitForFile(pidPath);
      controller.abort();
      await expect(pending).rejects.toThrow("Operation aborted");
    } finally {
      controller.abort();
      await pending.catch(() => undefined);
      if (priorPIDPath === undefined) {
        delete process.env.PI_WEB_TEST_PID_FILE;
      } else {
        process.env.PI_WEB_TEST_PID_FILE = priorPIDPath;
      }
      rmSync(directory, { recursive: true, force: true });
    }
  });
});
