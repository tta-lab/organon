import { Context } from "@deepseek-ai/cordis";
import { WebRuntime } from "@deepseek-ai/dsh-web";
import { describe, expect, it } from "vitest";

import { apply, SettingsSchema } from "../src/index.js";
import { createOrganonSearchProvider } from "../src/provider.js";
import { SEARCH_PROVIDER_ID, SETTINGS_NAMESPACE } from "../src/contract.js";

describe("DSH Web package composition", () => {
  it("registers the Organon provider and global Pi-compatible tools without replacing search", () => {
    let registered: any;
    let registeredNamespace = "";
    let registeredBase: unknown;
    const registeredTools: any[] = [];
    const ctx = {
      settings: {
        register: (namespace: string, schema: unknown, options: { base: unknown }) => {
          registeredNamespace = namespace;
          registeredBase = options.base;
          expect(schema).toBe(SettingsSchema);
          return { get: () => ({ provider: "brave" }) };
        },
      },
      credentials: { resolve: async () => undefined },
      web: {
        registerSearchProvider: (provider: unknown) => {
          registered = provider;
        },
      },
      tools: {
        register: (definition: unknown) => registeredTools.push(definition),
      },
    };

    apply(ctx as any);
    expect(registeredNamespace).toBe(SETTINGS_NAMESPACE);
    expect(registeredBase).toEqual({ provider: "exa" });
    expect(registered.id).toBe(SEARCH_PROVIDER_ID);
    expect(registered.available()).toBe(true);
    expect(registeredTools.map((definition) => definition.name)).toEqual([
      "web_fetch",
      "web_docs",
      "web_sgraph",
    ]);
    expect(registeredTools.every((definition) => definition.presentCall === undefined)).toBe(true);
    expect(registeredTools.every((definition) => definition.presentResult === undefined)).toBe(
      true,
    );
  });

  it("routes a PTC-shaped batch through the isolated rc.8 WebRuntime seam", async () => {
    const calls: Array<{ query: string; signal?: AbortSignal }> = [];
    const root = new Context();
    await root.plugin(WebRuntime, { searchProvider: SEARCH_PROVIDER_ID });
    const provider = createOrganonSearchProvider({
      binaryPath: "test-owned-web",
      getProvider: () => "exa",
      credentials: { resolve: async () => undefined },
      run: async (_binary, options) => {
        const marker = options.args.indexOf("--");
        calls.push({ query: options.args[marker + 1] ?? "", signal: options.signal });
        return {
          stdout: JSON.stringify({ provider: "Exa", results: [] }),
          stderr: "",
          exitCode: 0,
          killed: false,
        };
      },
    });
    await root.plugin({
      inject: ["web"],
      apply(ctx) {
        ctx.web.registerSearchProvider(provider);
      },
    });

    const controller = new AbortController();
    const results = await Promise.all(
      ["first query", "second query"].map((query) =>
        root.web.search({ query, maxResults: 8 }, controller.signal),
      ),
    );
    expect(results).toEqual([
      { sources: [], truncated: false },
      { sources: [], truncated: false },
    ]);
    expect(calls).toEqual([
      { query: "first query", signal: controller.signal },
      { query: "second query", signal: controller.signal },
    ]);
    await root.fiber.dispose();
  });
});
