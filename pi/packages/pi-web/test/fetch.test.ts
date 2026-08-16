import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

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

async function withTempHome<T>(fn: () => Promise<T>): Promise<T> {
  const prior = process.env.HOME;
  const home = mkdtempSync(join(tmpdir(), "pi-web-fetch-home-"));
  process.env.HOME = home;
  try {
    return await fn();
  } finally {
    if (prior === undefined) delete process.env.HOME;
    else process.env.HOME = prior;
    rmSync(home, { recursive: true, force: true });
  }
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
