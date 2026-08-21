import { createServer, type Server } from "node:http";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it, vi } from "vitest";

import { fetchWebPage } from "@tta-lab/pi-shared/fetch";

import { CONTEXT7_CREDENTIAL_REF } from "../src/contract.js";
import {
  createWebToolDefinitions,
  registerWebTools,
  type WebToolDependencies,
} from "../src/tools.js";

const emptySignal = () => new AbortController().signal;

type ToolDefinition = ReturnType<typeof createWebToolDefinitions>[number];

function callTool(
  definition: ToolDefinition,
  args: unknown,
  signal = emptySignal(),
): Promise<unknown> {
  return definition.execute(args, { signal } as never);
}

function dependencies(overrides: Partial<WebToolDependencies> = {}): WebToolDependencies {
  return {
    binaryPath: "/test-owned/web binary",
    credentials: { resolve: async () => undefined },
    ...overrides,
  };
}

async function startServer(): Promise<{ server: Server; url: string }> {
  const server = createServer((_request, response) => {
    response.setHeader("Content-Type", "text/plain");
    response.end(
      "# Test page\n\n## Install\nInstall content.\n\n### Details\nDetails content.\n\n## Next\nNext content.\n",
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("test server did not bind");
  return { server, url: `http://127.0.0.1:${address.port}/page` };
}

describe.sequential("DSH global Pi-compatible web tools", () => {
  it("registers the three global names with Pi-shaped schemas and generic presentation", () => {
    const definitions = createWebToolDefinitions(dependencies());
    const registered: ToolDefinition[] = [];
    registerWebTools(
      { tools: { register: (definition: ToolDefinition) => registered.push(definition) } } as never,
      dependencies(),
    );

    expect(definitions.map((definition) => definition.name)).toEqual([
      "web_fetch",
      "web_docs",
      "web_sgraph",
    ]);
    expect(registered.map((definition) => definition.name)).toEqual([
      "web_fetch",
      "web_docs",
      "web_sgraph",
    ]);
    const fetch = definitions[0]!;
    const docs = definitions[1]!;
    const sgraph = definitions[2]!;
    expect(Object.keys((fetch.parameters as any).properties)).toEqual([
      "url",
      "tree",
      "section_id",
      "full",
      "tree_threshold",
    ]);
    expect((fetch.parameters as any).additionalProperties).toBe(false);
    expect((docs.parameters as any).additionalProperties).toBe(false);
    expect((sgraph.parameters as any).additionalProperties).toBe(false);
    expect((docs.parameters as any).properties.action.enum).toEqual(["resolve", "fetch"]);
    expect(Object.keys((sgraph.parameters as any).properties)).toEqual([
      "query",
      "count",
      "context",
      "timeout",
    ]);
    expect((fetch.output.schema as any).properties).toMatchObject({
      url: { type: "string" },
      mode: { enum: ["full", "tree", "section"] },
      content: { type: "string" },
    });
    expect((fetch.output.schema as any).required).toEqual(["url", "mode", "content"]);
    expect((docs.output.schema as any).oneOf.map((branch: any) => branch.required)).toEqual([
      ["query", "libraries"],
      ["library_id", "content"],
    ]);
    expect((sgraph.output.schema as any).required).toEqual(["content"]);
    expect(fetch.presentCall).toBeUndefined();
    expect(fetch.presentResult).toBeUndefined();
    expect(fetch.output.presentationMeta).toBeUndefined();
    expect(docs.presentCall).toBeUndefined();
    expect(docs.presentResult).toBeUndefined();
    expect(docs.output.presentationMeta).toBeUndefined();
    expect(sgraph.presentCall).toBeUndefined();
    expect(sgraph.presentResult).toBeUndefined();
    expect(sgraph.output.presentationMeta).toBeUndefined();
  });

  it("bounds fetch navigation and docs/source rendering at the Pi model limits", () => {
    const [fetch, docs, sgraph] = createWebToolDefinitions(dependencies());
    const large = Array.from({ length: 3000 }, (_, index) => `line ${index}`).join("\n");
    for (const mode of ["tree", "section", "full"] as const) {
      const rendered = (
        fetch.output.render({}, { url: "https://example.test", mode, content: large })[0] as {
          text: string;
        }
      ).text;
      expect(rendered).toContain("[Truncated:");
      expect(rendered).not.toContain("line 2999");
      expect(rendered.split("\n").length).toBeLessThanOrEqual(2002);
    }
    const docsRendered = (
      docs.output.render({}, { library_id: "/org/lib", content: large })[0] as { text: string }
    ).text;
    const sourceRendered = (sgraph.output.render({}, { content: large })[0] as { text: string })
      .text;
    expect(docsRendered).toContain("[Truncated:");
    expect(sourceRendered).toContain("[Truncated:");
  });

  it("preserves shared fetch navigation arguments and structured results", async () => {
    const received: { input: unknown; signal?: AbortSignal }[] = [];
    const fetch = async (input: any, signal?: AbortSignal) => {
      received.push({ input, signal });
      return { url: input.url, mode: "section" as const, content: "selected section" };
    };
    const definition = createWebToolDefinitions(dependencies({ fetch }))[0]!;
    const controller = new AbortController();
    const result = await callTool(
      definition,
      {
        url: "https://example.test/docs?value=<>&space=yes",
        tree: true,
        section_id: "install",
        full: true,
        tree_threshold: 12,
      },
      controller.signal,
    );

    expect(received).toEqual([
      {
        input: {
          url: "https://example.test/docs?value=<>&space=yes",
          tree: true,
          section_id: "install",
          full: true,
          tree_threshold: 12,
        },
        signal: controller.signal,
      },
    ]);
    expect(result).toEqual({
      url: "https://example.test/docs?value=<>&space=yes",
      mode: "section",
      content: "selected section",
    });
  });

  it("uses the shared fetch runtime for tree, section, and full navigation", async () => {
    const { server, url } = await startServer();
    const cacheDirectory = await mkdtemp(join(tmpdir(), "dsh-web-tools-cache-"));
    try {
      const fetch = (input: any, signal?: AbortSignal) =>
        fetchWebPage(input, signal, { cacheDirectory });
      const definition = createWebToolDefinitions(dependencies({ fetch }))[0]!;
      const tree = (await callTool(definition, { url, tree: true })) as any;
      expect(tree.mode).toBe("tree");
      const sectionID = String(tree.content.match(/\[([^\]]+)\] ## Install/)?.[1] ?? "");
      expect(sectionID).not.toBe("");

      const section = (await callTool(definition, { url, section_id: sectionID })) as any;
      expect(section.mode).toBe("section");
      expect(section.content).toContain("Install content.");

      const full = (await callTool(definition, { url, full: true })) as any;
      expect(full.mode).toBe("full");
      expect(full.content).toContain("Details content.");
    } finally {
      await rm(cacheDirectory, { recursive: true, force: true });
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  });

  it("preserves docs actions, Sourcegraph controls, cancellation, and Context7 precedence", async () => {
    const secret = "ctx7sk-test-secret";
    const calls: Array<{ args: string[]; signal: AbortSignal; env?: NodeJS.ProcessEnv }> = [];
    const run = vi.fn(async (_binary: string, options: any) => {
      calls.push(options);
      if (options.args[0] === "docs" && options.args[1] === "resolve") {
        return {
          stdout: JSON.stringify({
            query: options.args[options.args.indexOf("--") + 1],
            libraries: [
              {
                id: "/org/lib",
                title: "Library",
                description: "A library",
                trust_score: 0.9,
                total_snippets: 4,
                versions: ["v1"],
              },
            ],
          }),
          stderr: "",
          exitCode: 0,
          killed: false,
        };
      }
      if (options.args[0] === "docs") {
        return {
          stdout: JSON.stringify({
            library_id: "/org/lib",
            topic: "hooks",
            content: "documentation",
          }),
          stderr: "",
          exitCode: 0,
          killed: false,
        };
      }
      return {
        stdout: JSON.stringify({ content: "# Sourcegraph results" }),
        stderr: "",
        exitCode: 0,
        killed: false,
      };
    });
    const credentials = {
      resolve: vi.fn(async (ref: string) => {
        expect(ref).toBe(CONTEXT7_CREDENTIAL_REF);
        return { value: secret, source: "file" };
      }),
    };
    const [docs, sgraph] = createWebToolDefinitions(dependencies({ credentials, run })).slice(1);
    const controller = new AbortController();
    const resolved = await callTool(
      docs!,
      { action: "resolve", query: "-dispatch-proof" },
      controller.signal,
    );
    const fetched = await callTool(
      docs!,
      { action: "fetch", library_id: "/-org/lib", topic: "-hooks", tokens: 500 },
      controller.signal,
    );
    const source = await callTool(
      sgraph!,
      { query: "-repo:tta-lab", count: 14, context: 3, timeout: 9 },
      controller.signal,
    );

    expect(resolved).toEqual({
      query: "-dispatch-proof",
      libraries: [
        {
          id: "/org/lib",
          title: "Library",
          description: "A library",
          trust_score: 0.9,
          total_snippets: 4,
          versions: ["v1"],
        },
      ],
    });
    expect(fetched).toEqual({ library_id: "/org/lib", topic: "hooks", content: "documentation" });
    expect(source).toEqual({ content: "# Sourcegraph results" });
    expect(calls.map((call) => call.args)).toEqual([
      ["docs", "resolve", "--json", "--", "-dispatch-proof"],
      ["docs", "fetch", "--tokens", "500", "--json", "--", "/-org/lib", "-hooks"],
      [
        "sgraph",
        "--json",
        "--count",
        "14",
        "--context",
        "3",
        "--timeout",
        "9",
        "--",
        "-repo:tta-lab",
      ],
    ]);
    expect(calls[0]!.env).toEqual({ CONTEXT7_API_KEY: secret });
    expect(calls[1]!.env).toEqual({ CONTEXT7_API_KEY: secret });
    expect(calls[2]!.env).toBeUndefined();
    expect(calls.every((call) => call.signal === controller.signal)).toBe(true);
    expect(JSON.stringify({ resolved, fetched, source })).not.toContain(secret);
  });

  it("leaves inherited Context7 fallback sources intact when DSH has no credential", async () => {
    const run = vi.fn(async (_binary: string, options: any) => ({
      stdout: JSON.stringify({ library_id: "/org/lib", content: "documentation" }),
      stderr: "",
      exitCode: 0,
      killed: false,
    }));
    const definition = createWebToolDefinitions(
      dependencies({ run, credentials: { resolve: async () => undefined } }),
    )[1]!;
    await callTool(definition, { action: "fetch", library_id: "/org/lib" });
    expect(run).toHaveBeenCalledWith(
      "/test-owned/web binary",
      expect.not.objectContaining({ env: expect.anything() }),
    );
  });

  it("redacts Context7 credentials from child failures", async () => {
    const secret = "ctx7sk-never-log-this";
    const definition = createWebToolDefinitions(
      dependencies({
        credentials: { resolve: async () => ({ value: secret, source: "file" }) },
        run: async () => ({
          stdout: "",
          stderr: `backend failed with ${secret}`,
          exitCode: 1,
          killed: false,
        }),
      }),
    )[1]!;

    await expect(callTool(definition, { action: "fetch", library_id: "/org/lib" })).rejects.toThrow(
      /web docs fetch failed|command exited with code 1/,
    );
    await expect(
      callTool(definition, { action: "fetch", library_id: "/org/lib" }),
    ).rejects.not.toThrow(secret);
  });

  it("rejects undeclared input fields before invoking the binary", async () => {
    const fetchRun = vi.fn();
    const fetchDefinition = createWebToolDefinitions(
      dependencies({ fetch: vi.fn(), run: fetchRun }),
    )[0]!;
    await expect(
      callTool(fetchDefinition, { url: "https://example.test", extra: true }),
    ).rejects.toThrow(/web_fetch input does not accept field extra/);
    expect(fetchRun).not.toHaveBeenCalled();

    const sgraphRun = vi.fn();
    const sgraphDefinition = createWebToolDefinitions(dependencies({ run: sgraphRun }))[2]!;
    await expect(callTool(sgraphDefinition, { query: "x", extra: true })).rejects.toThrow(
      /web_sgraph input does not accept field extra/,
    );
    expect(sgraphRun).not.toHaveBeenCalled();

    const run = vi.fn();
    const definition = createWebToolDefinitions(dependencies({ run }))[1]!;
    await expect(
      callTool(definition, { action: "resolve", query: "x", extra: true }),
    ).rejects.toThrow(/web_docs input does not accept field extra/);
    expect(run).not.toHaveBeenCalled();
  });

  it("rejects cross-action docs fields before invoking the binary", async () => {
    const run = vi.fn();
    const definition = createWebToolDefinitions(dependencies({ run }))[1]!;
    await expect(
      callTool(definition, { action: "resolve", query: "x", library_id: "/wrong" }),
    ).rejects.toThrow(/does not accept library_id/);
    expect(run).not.toHaveBeenCalled();
  });

  it("does not leak inherited fallback secrets from child failures", async () => {
    const secret = "ctx7sk-inherited-secret";
    const failing = createWebToolDefinitions(
      dependencies({
        credentials: { resolve: async () => undefined },
        run: async () => ({
          stdout: `malformed ${secret}`,
          stderr: `backend failed with ${secret}`,
          exitCode: 1,
          killed: false,
        }),
      }),
    )[1]!;
    await expect(
      callTool(failing, { action: "fetch", library_id: "/org/lib" }),
    ).rejects.not.toThrow(secret);

    const malformed = createWebToolDefinitions(
      dependencies({
        credentials: { resolve: async () => undefined },
        run: async () => ({
          stdout: `not-json ${secret}`,
          stderr: "",
          exitCode: 0,
          killed: false,
        }),
      }),
    )[1]!;
    await expect(
      callTool(malformed, { action: "fetch", library_id: "/org/lib" }),
    ).rejects.not.toThrow(secret);
  });
});
