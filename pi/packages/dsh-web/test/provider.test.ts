import { describe, expect, it } from "vitest";

import { BRAVE_CREDENTIAL_REF, EXA_CREDENTIAL_REF, SEARCH_PROVIDER_ID } from "../src/contract.js";
import { createOrganonSearchProvider } from "../src/provider.js";

const resultJSON = JSON.stringify({
  provider: "Brave",
  results: [{ title: "One", link: "https://example.com/one", snippet: "First", position: 1 }],
});

function fakePtcSearch(
  web: {
    search(request: { query: string; maxResults?: number }, signal?: AbortSignal): Promise<unknown>;
  },
  queries: readonly string[],
  signal?: AbortSignal,
): Promise<unknown[]> {
  return Promise.all(queries.map((query) => web.search({ query, maxResults: 8 }, signal)));
}

describe("Organon DSH search provider", () => {
  it("routes the PTC-shaped search call through the selected provider, DSH env override, and result mapping", async () => {
    let received: { binary: string; options: any } | undefined;
    const provider = createOrganonSearchProvider({
      binaryPath: "/tmp/organon web binary/web",
      getProvider: () => "brave",
      credentials: {
        resolve: async (ref) => {
          expect(ref).toBe(BRAVE_CREDENTIAL_REF);
          return { value: "dsh-secret", source: "file" };
        },
      },
      run: async (binary, options) => {
        received = { binary, options };
        return { stdout: resultJSON, stderr: "", exitCode: 0, killed: false };
      },
    });
    const registered = new Map([[SEARCH_PROVIDER_ID, provider]]);
    const web = {
      search: async (request: { query: string; maxResults?: number }, signal?: AbortSignal) =>
        registered.get(SEARCH_PROVIDER_ID)!.search(request, signal),
    };
    const controller = new AbortController();
    const flagLikeQuery = "--flag-like query";

    const results = await fakePtcSearch(web, [flagLikeQuery], controller.signal);
    expect(provider.id).toBe(SEARCH_PROVIDER_ID);
    expect(provider.available()).toBe(true);
    expect(received?.binary).toContain("organon web binary");
    expect(received?.options.args).toEqual([
      "search",
      "--provider",
      "brave",
      "--json",
      "--",
      flagLikeQuery,
    ]);
    expect(received?.options.env).toEqual({ BRAVE_API_KEY: "dsh-secret" });
    expect(received?.options.signal).toBe(controller.signal);
    expect(results).toEqual([
      {
        sources: [{ url: "https://example.com/one", title: "One", snippet: "First" }],
        truncated: false,
      },
    ]);
  });

  it("preserves inherited environment and ttal dotenv fallback when DSH has no credential", async () => {
    let receivedOptions: any;
    let childEnvironment: Record<string, string | undefined> | undefined;
    const provider = createOrganonSearchProvider({
      binaryPath: "web",
      getProvider: () => "exa",
      credentials: {
        resolve: async (ref) => {
          expect(ref).toBe(EXA_CREDENTIAL_REF);
          return undefined;
        },
      },
      run: async (_binary, options) => {
        receivedOptions = options;
        const dotenv = { EXA_API_KEY: "dotenv-value" };
        const inherited = { EXA_API_KEY: "inherited-value" };
        childEnvironment = { ...dotenv, ...inherited, ...options.env };
        return { stdout: resultJSON, stderr: "", exitCode: 0, killed: false };
      },
    });

    await provider.search({ query: "fallback" });
    expect(receivedOptions.args).toEqual([
      "search",
      "--provider",
      "exa",
      "--json",
      "--",
      "fallback",
    ]);
    expect(receivedOptions.env).toBeUndefined();
    expect(childEnvironment?.EXA_API_KEY).toBe("inherited-value");
  });

  it("does not resolve a DuckDuckGo credential and forwards cancellation", async () => {
    let receivedSignal: AbortSignal | undefined;
    let credentialCalls = 0;
    const provider = createOrganonSearchProvider({
      binaryPath: "web",
      getProvider: () => "duckduckgo",
      credentials: {
        resolve: async () => {
          credentialCalls++;
          return undefined;
        },
      },
      run: async (_binary, options) => {
        receivedSignal = options.signal;
        return { stdout: resultJSON, stderr: "", exitCode: 0, killed: false };
      },
    });
    const controller = new AbortController();

    await provider.search({ query: "cancel", maxResults: 1 }, controller.signal);
    expect(receivedSignal).toBe(controller.signal);
    expect(credentialCalls).toBe(0);
  });

  it("rejects malformed JSON and invalid source fields before returning an rc.8 result", async () => {
    const malformed = createOrganonSearchProvider({
      binaryPath: "web",
      getProvider: () => "exa",
      credentials: { resolve: async () => undefined },
      run: async () => ({ stdout: "{not-json", stderr: "", exitCode: 0, killed: false }),
    });
    await expect(malformed.search({ query: "malformed" })).rejects.toThrow(/invalid JSON/);

    for (const [field, value] of [
      ["link", 42],
      ["title", 42],
      ["snippet", 42],
    ] as const) {
      const invalid = createOrganonSearchProvider({
        binaryPath: "web",
        getProvider: () => "exa",
        credentials: { resolve: async () => undefined },
        run: async () => ({
          stdout: JSON.stringify({
            results: [
              { link: "https://example.com", title: "Title", snippet: "Snippet", [field]: value },
            ],
          }),
          stderr: "",
          exitCode: 0,
          killed: false,
        }),
      });
      await expect(invalid.search({ query: `invalid-${field}` })).rejects.toThrow(
        new RegExp(`${field} must be a string`),
      );
    }
  });

  it("reports child failures without exposing credential values", async () => {
    const provider = createOrganonSearchProvider({
      binaryPath: "web",
      getProvider: () => "exa",
      credentials: { resolve: async () => ({ value: "never-returned", source: "file" }) },
      run: async () => ({
        stdout: "",
        stderr: "provider unavailable never-returned",
        exitCode: 1,
        killed: false,
      }),
    });

    await expect(provider.search({ query: "failed" })).rejects.toThrow(/exa search failed/);
    await expect(provider.search({ query: "failed" })).rejects.not.toThrow("never-returned");
  });
});
