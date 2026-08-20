import { createServer } from "node:http";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it, vi } from "vitest";

vi.mock("defuddle/node", () => ({
  Defuddle: vi.fn(async () => {
    throw new Error("synthetic parser failure");
  }),
}));

import { fetchWebPage } from "../src/fetch.js";

describe("native Pi fetch parser failures", () => {
  it("reports Defuddle parse failures with the existing fetch error surface", async () => {
    const server = createServer((_req, res) => {
      res.setHeader("Content-Type", "text/html; charset=utf-8");
      res.end("<html><body><article>content</article></body></html>");
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("server did not bind");

    const priorHome = process.env.HOME;
    const priorXDG = process.env.XDG_CACHE_HOME;
    const priorLocalAppData = process.env.LOCALAPPDATA;
    const home = mkdtempSync(join(tmpdir(), "pi-web-parse-failure-"));
    process.env.HOME = home;
    delete process.env.XDG_CACHE_HOME;
    delete process.env.LOCALAPPDATA;
    try {
      await expect(fetchWebPage({ url: `http://127.0.0.1:${address.port}` })).rejects.toThrow(
        /defuddle parse failed: synthetic parser failure/,
      );
    } finally {
      if (priorHome === undefined) delete process.env.HOME;
      else process.env.HOME = priorHome;
      if (priorXDG === undefined) delete process.env.XDG_CACHE_HOME;
      else process.env.XDG_CACHE_HOME = priorXDG;
      if (priorLocalAppData === undefined) delete process.env.LOCALAPPDATA;
      else process.env.LOCALAPPDATA = priorLocalAppData;
      rmSync(home, { recursive: true, force: true });
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });
});
