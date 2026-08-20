import { existsSync } from "node:fs";
import { cp, mkdir, mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { Context } from "@deepseek-ai/cordis";
import { WebRuntime } from "@deepseek-ai/dsh-web";
import { parse } from "yaml";
import { describe, expect, it } from "vitest";

const packageRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const packageNodeModules = join(packageRoot, "node_modules");

interface ProfileRow {
  id: string;
  name: string;
  config?: Record<string, unknown>;
  disabled?: boolean;
  inject?: string[];
}

const STOCK_PROFILE = {
  name: "web",
  preset: "code",
  rows: [
    {
      id: "web",
      name: "@deepseek-ai/dsh-web",
      config: { searchProvider: "deepseek-official" },
    },
    {
      id: "tool-web",
      name: "stock-url-tool-web",
      config: { fetch: true },
    },
    {
      id: "code-ptc",
      name: "stock-code-ptc",
    },
  ] satisfies ProfileRow[],
};

function applyBundlePatch(
  rows: readonly ProfileRow[],
  patch: readonly Record<string, unknown>[],
): ProfileRow[] {
  const composed = rows.map((row) => ({ ...row }));
  const positions = new Map(composed.map((row, index) => [row.id, index]));
  const inserted: ProfileRow[] = [];
  for (const entry of patch) {
    if (Array.isArray(entry.insert)) {
      inserted.push(...(entry.insert as unknown as ProfileRow[]));
      continue;
    }
    const id = typeof entry.id === "string" ? entry.id : undefined;
    const index = id === undefined ? undefined : positions.get(id);
    if (index === undefined)
      throw new Error(`patch row ${id ?? "<missing id>"} is not in stock profile`);
    composed[index] = { ...composed[index], ...(entry as unknown as ProfileRow) };
  }
  return [...composed, ...inserted];
}

async function installBuiltPackage(home: string): Promise<{
  profile: string;
  installed: string;
  patch: readonly Record<string, unknown>[];
  manifest: Record<string, any>;
}> {
  const profile = join(home, "profiles", "web");
  const installed = join(profile, "node_modules", "@tta-lab", "dsh-web");
  await mkdir(join(installed, "dist"), { recursive: true });
  await symlink(packageNodeModules, join(installed, "node_modules"), "dir");
  for (const file of ["package.json", "README.md", "cordis.patch.yml"]) {
    await cp(join(packageRoot, file), join(installed, file));
  }
  for (const file of ["index.js", "client.js", "index.d.ts", "client.d.cts"]) {
    const source = join(packageRoot, "dist", file);
    if (!existsSync(source)) throw new Error(`build artifact is missing: ${source}`);
    await cp(source, join(installed, "dist", file));
  }
  const manifest = JSON.parse(await readFile(join(installed, "package.json"), "utf8")) as Record<
    string,
    any
  >;
  const patch = parse(
    await readFile(join(installed, "cordis.patch.yml"), "utf8"),
  ) as readonly Record<string, unknown>[];
  await writeFile(
    join(profile, "profile.json"),
    JSON.stringify({ name: "web", preset: "code", packages: [manifest.name] }, null, 2) + "\n",
  );
  return { profile, installed, patch, manifest };
}

interface ToolRegistry {
  register(name: string, execute: (args: any, signal?: AbortSignal) => Promise<unknown>): void;
  get(name: string): ((args: any, signal?: AbortSignal) => Promise<unknown>) | undefined;
}

function toolRegistry(): ToolRegistry {
  const handlers = new Map<string, (args: any, signal?: AbortSignal) => Promise<unknown>>();
  return {
    register(name, execute) {
      if (handlers.has(name)) throw new Error(`duplicate test tool ${name}`);
      handlers.set(name, execute);
    },
    get(name) {
      return handlers.get(name);
    },
  };
}

describe.sequential("DSH rc.8 normal Web profile composition", () => {
  it("installs the built package into a test-owned Web profile and routes native PTC search", async () => {
    const home = await mkdtemp(join(tmpdir(), "dsh rc8 composition "));
    const runnerWorkspace = join(home, "runner workspace");
    const binary = join(runnerWorkspace, "packages", "native", "pi-web-darwin-arm64", "bin", "web");
    const recordPath = join(home, "organon child records.jsonl");
    const previousWorkspace = process.env.ORGANON_PI_TEST_WORKSPACE;
    const previousVitest = process.env.VITEST;
    let root: Context | undefined;
    try {
      process.env.ORGANON_PI_TEST_WORKSPACE = runnerWorkspace;
      process.env.VITEST = "true";
      await mkdir(dirname(binary), { recursive: true });
      const fakeBinary = [
        "#!/usr/bin/env node",
        'import { appendFileSync } from "node:fs";',
        `const recordPath = ${JSON.stringify(recordPath)};`,
        "const args = process.argv.slice(2);",
        'const provider = args[args.indexOf("--provider") + 1];',
        'const query = args[args.indexOf("--") + 1];',
        "appendFileSync(recordPath, JSON.stringify({ args, key: process.env.BRAVE_API_KEY ?? null }) + String.fromCharCode(10));",
        'process.stdout.write(JSON.stringify({ provider, results: [{ link: "https://example.test/" + encodeURIComponent(query), title: "Organon", snippet: "fake result", position: 1 }] }));',
      ].join("\n");
      await writeFile(binary, fakeBinary, { mode: 0o755 });

      const { profile, installed, patch, manifest } = await installBuiltPackage(home);
      const profileDocument = JSON.parse(await readFile(join(profile, "profile.json"), "utf8")) as {
        name: string;
        preset: string;
        packages: string[];
      };
      const rows = applyBundlePatch(STOCK_PROFILE.rows, patch);
      const webRow = rows.find((row) => row.id === "web");
      const rootToolRow = rows.find((row) => row.id === "tool-web");
      const hostRow = rows.find((row) => row.id === "organon-dsh-web");
      expect(profileDocument).toEqual({
        name: "web",
        preset: "code",
        packages: ["@tta-lab/dsh-web"],
      });
      expect(webRow).toEqual({
        id: "web",
        name: "@deepseek-ai/dsh-web",
        config: { searchProvider: "organon-web-search" },
      });
      expect(rootToolRow?.disabled).toBe(true);
      expect(hostRow).toMatchObject({
        name: "@tta-lab/dsh-web",
        inject: ["web", "credentials", "settings"],
      });
      expect(manifest.dsh.client.platform).toBe("web");
      expect(await readFile(join(installed, "dist", "client.js"), "utf8")).toContain(
        "window.__ModuleLoader__.load",
      );

      const tools = toolRegistry();
      root = new Context();
      root.provide("tools", tools as any);
      root.provide("settings", {
        register: () => ({ get: () => ({ provider: "brave" }) }),
      } as any);
      root.provide("credentials", {
        resolve: async () => ({ value: "dsh-brave-secret", source: "file" }),
      } as any);

      for (const row of rows) {
        if (row.disabled) continue;
        if (row.name === "@deepseek-ai/dsh-web") {
          await root.plugin(WebRuntime, webRow?.config as any);
        } else if (row.name === "stock-url-tool-web") {
          await root.plugin({
            inject: ["tools"],
            apply(ctx: any) {
              ctx.tools.register("web_fetch", async (args: any) => ({
                url: args.url,
                content: "stock URL-only fetch",
              }));
            },
          } as any);
        } else if (row.name === "stock-code-ptc") {
          await root.plugin({
            inject: ["tools", "web"],
            apply(ctx: any) {
              ctx.tools.register("web_search", async (args: any, signal?: AbortSignal) => {
                if (!Array.isArray(args.queries)) throw new Error("stock PTC requires queries[]");
                return Promise.all(
                  args.queries.map((query: string) =>
                    ctx.web.search({ query, maxResults: 8 }, signal),
                  ),
                );
              });
            },
          } as any);
        } else if (row.name === "@tta-lab/dsh-web") {
          const host = await import(
            `${pathToFileURL(join(installed, "dist", "index.js")).href}?composition=${Date.now()}`
          );
          await root.plugin({ name: host.name, inject: host.inject, apply: host.apply } as any);
        }
      }

      const search = tools.get("web_search");
      expect(search).toBeDefined();
      expect(tools.get("web_fetch")).toBeUndefined();
      const controller = new AbortController();
      const results = (await search!(
        { queries: ["-flag-like query", "normal query"] },
        controller.signal,
      )) as Array<{ sources: Array<{ title?: string }> }>;
      expect(results).toHaveLength(2);
      expect(results.every((result: any) => result.sources[0].title === "Organon")).toBe(true);

      const records = (await readFile(recordPath, "utf8"))
        .trim()
        .split("\n")
        .filter(Boolean)
        .map((line) => JSON.parse(line) as { args: string[]; key: string | null });
      expect(records).toHaveLength(2);
      const byQuery = new Map(
        records.map((record) => [record.args[record.args.indexOf("--") + 1], record]),
      );
      for (const query of ["-flag-like query", "normal query"]) {
        expect(byQuery.get(query)).toEqual({
          args: ["search", "--provider", "brave", "--json", "--", query],
          key: "dsh-brave-secret",
        });
      }
    } finally {
      if (root !== undefined) await root.fiber.dispose();
      if (previousWorkspace === undefined) delete process.env.ORGANON_PI_TEST_WORKSPACE;
      else process.env.ORGANON_PI_TEST_WORKSPACE = previousWorkspace;
      if (previousVitest === undefined) delete process.env.VITEST;
      else process.env.VITEST = previousVitest;
      await rm(home, { recursive: true, force: true });
    }
  });
});
