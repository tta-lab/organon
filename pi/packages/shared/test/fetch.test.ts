import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { mkdirSync, mkdtempSync, readdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
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
  const priorHome = process.env.HOME;
  const priorXDG = process.env.XDG_CACHE_HOME;
  const priorLocalAppData = process.env.LOCALAPPDATA;
  const home = mkdtempSync(join(tmpdir(), "pi-web-fetch-home-"));
  process.env.HOME = home;
  delete process.env.XDG_CACHE_HOME;
  delete process.env.LOCALAPPDATA;
  try {
    return await fn(home);
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

function resolveCacheDirectory(home: string): string {
  return process.platform === "win32"
    ? join(home, "AppData", "Local", "organon", "scrapes")
    : join(home, ".cache", "organon", "scrapes");
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

  it("hits same-day entries, misses stale entries, and hashes URL cache names", async () => {
    let requests = 0;
    const { server, url } = await startServer((req, res) => {
      requests++;
      res.setHeader("Content-Type", "text/plain");
      res.end(`network ${req.url}`);
    });
    try {
      await withTempHome(async (home) => {
        const cachedURL = `${url}/cached`;
        await fetchWebPage({ url: cachedURL });
        await fetchWebPage({ url: cachedURL });
        expect(requests).toBe(1);

        const cacheDirectory = resolveCacheDirectory(home);
        let files = readdirSync(cacheDirectory);
        expect(files).toHaveLength(1);
        expect(files[0]).toMatch(/^[0-9a-f]{64}__[0-9]{4}-[0-9]{2}-[0-9]{2}[.]md$/);

        const staleDate = new Date(Date.now() - 24 * 60 * 60 * 1000);
        const staleDay = [staleDate.getFullYear(), staleDate.getMonth() + 1, staleDate.getDate()]
          .map((part, index) => (index === 0 ? String(part) : String(part).padStart(2, "0")))
          .join("-");
        const currentPath = join(cacheDirectory, files[0]!);
        const staleName = files[0]!.replace(
          /__[0-9]{4}-[0-9]{2}-[0-9]{2}[.]md$/,
          `__${staleDay}.md`,
        );
        renameSync(currentPath, join(cacheDirectory, staleName));
        await fetchWebPage({ url: cachedURL });
        expect(requests).toBe(2);

        const windowsURL = `${url}/windows?value=%3C%3E%3A%22%7C%3F*`;
        await fetchWebPage({ url: windowsURL });
        await fetchWebPage({ url: `${url}/long?${"value=".repeat(400)}` });
        await fetchWebPage({ url: `${url}/query?value=one` });
        await fetchWebPage({ url: `${url}/query?value=two` });
        await fetchWebPage({ url: `${url}/query?value=one` });
        expect(requests).toBe(6);

        files = readdirSync(cacheDirectory);
        expect(
          files.every((file) => /^[0-9a-f]{64}__[0-9]{4}-[0-9]{2}-[0-9]{2}[.]md$/.test(file)),
        ).toBe(true);
        expect(files.every((file) => file.length <= 80)).toBe(true);
      });
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });
  it("continues when cache setup, read, or write fails", async () => {
    let requests = 0;
    const { server, url } = await startServer((_req, res) => {
      requests++;
      res.setHeader("Content-Type", "text/plain");
      res.end("cache failure fallback");
    });
    const priorHome = process.env.HOME;
    const priorXDG = process.env.XDG_CACHE_HOME;
    const priorLocalAppData = process.env.LOCALAPPDATA;
    const homeFile = mkdtempSync(join(tmpdir(), "pi-web-cache-file-"));
    let cacheHome: string | undefined;
    const invalidHome = join(homeFile, "home-file");
    writeFileSync(invalidHome, "not a directory");
    delete process.env.XDG_CACHE_HOME;
    delete process.env.LOCALAPPDATA;
    try {
      process.env.HOME = invalidHome;
      await fetchWebPage({ url: `${url}/setup-failure` });
      await fetchWebPage({ url: `${url}/setup-failure` });
      expect(requests).toBe(2);

      cacheHome = mkdtempSync(join(tmpdir(), "pi-web-cache-read-"));
      process.env.HOME = cacheHome;
      const cacheDirectory = resolveCacheDirectory(cacheHome);
      const brokenURL = `${url}/read-write-failure`;
      await fetchWebPage({ url: brokenURL });
      const cachedFile = readdirSync(cacheDirectory)[0];
      if (!cachedFile) throw new Error("cache file was not created");
      const brokenPath = join(cacheDirectory, cachedFile);
      rmSync(brokenPath, { force: true });
      mkdirSync(brokenPath, { recursive: true });
      await fetchWebPage({ url: brokenURL });
      await fetchWebPage({ url: brokenURL });
      expect(requests).toBe(5);
    } finally {
      if (priorHome === undefined) delete process.env.HOME;
      else process.env.HOME = priorHome;
      if (priorXDG === undefined) delete process.env.XDG_CACHE_HOME;
      else process.env.XDG_CACHE_HOME = priorXDG;
      if (priorLocalAppData === undefined) delete process.env.LOCALAPPDATA;
      else process.env.LOCALAPPDATA = priorLocalAppData;
      rmSync(homeFile, { recursive: true, force: true });
      if (cacheHome) rmSync(cacheHome, { recursive: true, force: true });
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });
  it("preserves the HTML extraction trailing newline", async () => {
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
