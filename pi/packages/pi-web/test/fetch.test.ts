import { createHash } from "node:crypto";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it, vi } from "vitest";

import { fetchWebPage } from "../src/fetch.js";
import { renderMarkdown } from "../src/markdown.js";

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

async function withTempHome<T>(fn: (home: string) => Promise<T>): Promise<T> {
  const prior = process.env.HOME;
  const home = mkdtempSync(join(tmpdir(), "pi-web-fetch-home-"));
  process.env.HOME = home;
  try {
    return await fn(home);
  } finally {
    if (prior === undefined) delete process.env.HOME;
    else process.env.HOME = prior;
    rmSync(home, { recursive: true, force: true });
  }
}

function cachePath(home: string, url: string, date = new Date()): string {
  const parsed = new URL(url);
  let base = url.replaceAll("://", "___");
  if (parsed.search) base = base.split("?", 1)[0]!;
  base = base.replaceAll("/", "_").replace(/_$/, "").replaceAll("..", "__");
  if (parsed.search) {
    const queryHash = createHash("sha256").update(parsed.search.slice(1)).digest("hex").slice(0, 8);
    base += `_q${queryHash}`;
  }
  if (base.length > 200) {
    const hash = createHash("sha256").update(base).digest("hex").slice(0, 8);
    base = base.slice(0, 191) + `_${hash}`;
  }
  const day = [date.getFullYear(), date.getMonth() + 1, date.getDate()]
    .map((part, index) => (index === 0 ? String(part) : String(part).padStart(2, "0")))
    .join("-");
  return join(home, ".cache", "organon", "scrapes", `${base}__${day}.md`);
}

describe.sequential("native Pi fetch", () => {
  it("follows redirects, ignores BROWSER_GATEWAY_URL, and caches successful fetches", async () => {
    let requests = 0;
    const { server, url } = await startServer((req, res) => {
      requests++;
      if (req.url === "/redirect") {
        res.statusCode = 302;
        res.setHeader("Location", "/page");
        res.end();
        return;
      }
      res.setHeader("Content-Type", "text/plain");
      res.end("cached text");
    });
    const priorGateway = process.env.BROWSER_GATEWAY_URL;
    process.env.BROWSER_GATEWAY_URL = "http://127.0.0.1:1/should-not-be-used";
    try {
      await withTempHome(async () => {
        const first = await fetchWebPage({ url: `${url}/redirect` });
        const second = await fetchWebPage({ url: `${url}/redirect` });
        expect(first).toEqual({ url: `${url}/redirect`, mode: "full", content: "cached text" });
        expect(second).toEqual(first);
        expect(requests).toBe(2);
      });
    } finally {
      if (priorGateway === undefined) delete process.env.BROWSER_GATEWAY_URL;
      else process.env.BROWSER_GATEWAY_URL = priorGateway;
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("accepts final 3xx responses and rejects only 4xx/5xx responses", async () => {
    const originalFetch = globalThis.fetch;
    const statuses = [399, 400, 503];
    let status = 399;
    globalThis.fetch = vi.fn(
      async () =>
        new Response("status body", {
          status,
          headers: { "Content-Type": "text/plain" },
        }),
    ) as typeof fetch;
    try {
      await withTempHome(async () => {
        await expect(fetchWebPage({ url: "http://status.test/399" })).resolves.toMatchObject({
          content: "status body",
        });
        for (const rejectedStatus of statuses.slice(1)) {
          status = rejectedStatus;
          await expect(
            fetchWebPage({ url: `http://status.test/${rejectedStatus}` }),
          ).rejects.toThrow(`HTTP ${rejectedStatus}`);
        }
      });
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it("reuses compatible cache entries, expires stale entries, and separates queries", async () => {
    let requests = 0;
    const { server, url } = await startServer((req, res) => {
      requests++;
      res.setHeader("Content-Type", "text/plain");
      res.end(`network ${req.url}`);
    });
    try {
      await withTempHome(async (home) => {
        const cachedURL = `${url}/cached`;
        mkdirSync(join(home, ".cache", "organon", "scrapes"), { recursive: true });
        writeFileSync(cachePath(home, cachedURL), "Go-compatible cache");
        await expect(fetchWebPage({ url: cachedURL })).resolves.toMatchObject({
          content: "Go-compatible cache",
        });
        expect(requests).toBe(0);

        const staleURL = `${url}/stale`;
        writeFileSync(
          cachePath(home, staleURL, new Date(Date.now() - 24 * 60 * 60 * 1000)),
          "stale cache",
        );
        await expect(fetchWebPage({ url: staleURL })).resolves.toMatchObject({
          content: `network /stale`,
        });
        expect(requests).toBe(1);

        await fetchWebPage({ url: `${url}/query?value=one` });
        await fetchWebPage({ url: `${url}/query?value=two` });
        await fetchWebPage({ url: `${url}/query?value=one` });
        expect(requests).toBe(3);
      });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("continues without caching when setup, read, or write fails", async () => {
    let requests = 0;
    const { server, url } = await startServer((_req, res) => {
      requests++;
      res.setHeader("Content-Type", "text/plain");
      res.end("cache failure fallback");
    });
    const priorHome = process.env.HOME;
    const homeFile = mkdtempSync(join(tmpdir(), "pi-web-cache-file-"));
    let cacheHome: string | undefined;
    const invalidHome = join(homeFile, "home-file");
    writeFileSync(invalidHome, "not a directory");
    try {
      process.env.HOME = invalidHome;
      await fetchWebPage({ url: `${url}/setup-failure` });
      await fetchWebPage({ url: `${url}/setup-failure` });
      expect(requests).toBe(2);

      cacheHome = mkdtempSync(join(tmpdir(), "pi-web-cache-read-"));
      process.env.HOME = cacheHome;
      const cacheDirectory = join(cacheHome, ".cache", "organon", "scrapes");
      const brokenPath = cachePath(cacheHome, `${url}/read-write-failure`);
      mkdirSync(cacheDirectory, { recursive: true });
      mkdirSync(brokenPath, { recursive: true });
      await fetchWebPage({ url: `${url}/read-write-failure` });
      await fetchWebPage({ url: `${url}/read-write-failure` });
      expect(requests).toBe(4);
    } finally {
      if (priorHome === undefined) delete process.env.HOME;
      else process.env.HOME = priorHome;
      rmSync(homeFile, { recursive: true, force: true });
      if (cacheHome) rmSync(cacheHome, { recursive: true, force: true });
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("preserves the Go HTML extraction trailing newline", async () => {
    const { server, url } = await startServer((_req, res) => {
      res.setHeader("Content-Type", "text/html; charset=utf-8");
      res.end("<html><body><article><p>Extracted text.</p></article></body></html>");
    });
    try {
      await withTempHome(async () => {
        const result = await fetchWebPage({ url, full: true });
        expect(result.content).toBe("Extracted text.\n");
      });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("rejects binary media and binary bodies with actionable errors", async () => {
    const { server, url } = await startServer((req, res) => {
      if (req.url === "/body") {
        res.setHeader("Content-Type", "text/plain");
        res.end(Buffer.from([120, 0, 121]));
        return;
      }
      res.setHeader("Content-Type", "application/pdf");
      res.end("not really a pdf");
    });
    try {
      await withTempHome(async () => {
        await expect(fetchWebPage({ url })).rejects.toThrow(/binary content/);
        await expect(fetchWebPage({ url: `${url}/body` })).rejects.toThrow(/curl -L -O/);
      });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("stops a caller-cancelled request", async () => {
    const { server, url } = await startServer((_req, res) => {
      setTimeout(() => res.end("late"), 500);
    });
    try {
      await withTempHome(async () => {
        const controller = new AbortController();
        const pending = fetchWebPage({ url }, controller.signal);
        controller.abort();
        await expect(pending).rejects.toThrow("Operation aborted");
      });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("normalizes request timeout without waiting 30 seconds", async () => {
    const { server, url } = await startServer((_req, _res) => {
      // Keep the response open until the request signal aborts it.
    });
    const realSetTimeout = globalThis.setTimeout;
    const timeoutSpy = vi
      .spyOn(globalThis, "setTimeout")
      .mockImplementation(((handler: (...args: any[]) => void, timeout?: number, ...args: any[]) =>
        realSetTimeout(handler, timeout === 30_000 ? 0 : timeout, ...args)) as typeof setTimeout);
    try {
      await withTempHome(async () => {
        await expect(fetchWebPage({ url: `${url}/slow` })).rejects.toThrow(
          "fetch timed out after 30 seconds",
        );
      });
    } finally {
      timeoutSpy.mockRestore();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("rejects responses over the 10 MiB body limit", async () => {
    const { server, url } = await startServer((_req, res) => {
      res.setHeader("Content-Type", "text/plain");
      res.end("x".repeat(10 * 1024 * 1024 + 1));
    });
    try {
      await withTempHome(async () => {
        await expect(fetchWebPage({ url })).rejects.toThrow(/10485760 byte limit/);
      });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("preserves heading IDs, section boundaries, tree modes, and full precedence", () => {
    const source =
      "# Test page\n\n## Install\nInstall content.\n\n### Details\nDetails content.\n\n## Next\nNext content.\n";
    const tree = renderMarkdown(source, true, undefined, false, 5000);
    expect(tree.mode).toBe("tree");
    expect(tree.content).toContain("[7i] ## Install");
    expect(tree.content).toContain("[eD] ### Details");

    const section = renderMarkdown(source, false, "7i", false, 5000);
    expect(section.mode).toBe("section");
    expect(section.content).toBe("## Install\nInstall content.\n\n### Details\nDetails content.\n");

    const full = renderMarkdown("# H\n\n" + "x".repeat(6000), false, undefined, true, 5000);
    expect(full.mode).toBe("full");
    expect(full.content).toContain("x".repeat(100));
  });
});
