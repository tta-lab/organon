import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { Context } from "@deepseek-ai/cordis";
import { WebRuntime } from "@deepseek-ai/dsh-web";
import { parse } from "yaml";
import { describe, expect, it } from "vitest";

import { apply, SettingsSchema } from "../src/index.js";
import { createOrganonSearchProvider } from "../src/provider.js";
import { SEARCH_PROVIDER_ID, SETTINGS_NAMESPACE } from "../src/contract.js";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");

describe("DSH Web package composition", () => {
  it("registers the Organon provider and does not register a model-facing root tool", () => {
    let registered: any;
    let registeredNamespace = "";
    let registeredBase: unknown;
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
    };

    apply(ctx as any);
    expect(registeredNamespace).toBe(SETTINGS_NAMESPACE);
    expect(registeredBase).toEqual({ provider: "duckduckgo" });
    expect(registered.id).toBe(SEARCH_PROVIDER_ID);
    expect(registered.available()).toBe(true);
  });

  it("routes a PTC-shaped batch through the isolated rc.8 WebRuntime seam", async () => {
    const calls: Array<{ query: string; signal?: AbortSignal }> = [];
    const root = new Context();
    await root.plugin(WebRuntime, { searchProvider: SEARCH_PROVIDER_ID });
    const provider = createOrganonSearchProvider({
      binaryPath: "test-owned-web",
      getProvider: () => "duckduckgo",
      credentials: { resolve: async () => undefined },
      run: async (_binary, options) => {
        const marker = options.args.indexOf("--");
        calls.push({ query: options.args[marker + 1] ?? "", signal: options.signal });
        return {
          stdout: JSON.stringify({ provider: "DuckDuckGo", results: [] }),
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

  it("declares the rc.8 dual-face artifact, official settings injects, and complete stock Web patch", () => {
    const manifest = JSON.parse(readFileSync(join(root, "package.json"), "utf8"));
    const patch = parse(readFileSync(join(root, "cordis.patch.yml"), "utf8")) as any[];
    expect(manifest.name).toBe("@tta-lab/dsh-web");
    expect(manifest.peerDependencies["@deepseek-ai/dsh-web"]).toBe("0.1.0-rc.8");
    expect(manifest.dsh.bundle.patch).toBe("./cordis.patch.yml");
    expect(manifest.dsh.client.platform).toBe("web");
    expect(manifest.dsh.client.inject).toEqual([
      "@deepseek-ai/dsh-api-remotes",
      "@deepseek-ai/dsh-client-connection",
      "@deepseek-ai/dsh-client-locale",
      "@deepseek-ai/dsh-client-runtime",
      "@deepseek-ai/dsh-client-ui-settings",
      "@deepseek-ai/dsh-client-ui-settings-plugins",
    ]);
    expect(manifest.exports["./client"]).toEqual({
      types: "./dist/client.d.cts",
      default: "./dist/client.js",
    });

    expect(patch.find((entry) => entry.id === "web")).toEqual({
      id: "web",
      name: "@deepseek-ai/dsh-web",
      config: { searchProvider: SEARCH_PROVIDER_ID },
    });
    expect(patch.find((entry) => entry.id === "tool-web")).toEqual({
      id: "tool-web",
      disabled: true,
    });
    const inserted = patch.find((entry) => Array.isArray(entry.insert))?.insert ?? [];
    expect(inserted).toContainEqual({
      id: "organon-dsh-web",
      name: "@tta-lab/dsh-web",
      inject: ["web", "credentials", "settings"],
    });
    expect(JSON.stringify(patch)).not.toContain("web_search");
    expect(JSON.stringify(patch)).not.toContain("preset");
  });
});
