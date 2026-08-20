import { existsSync } from "node:fs";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { createRequire } from "node:module";
import { mkdir, mkdtemp, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { describe, expect, it } from "vitest";

const packageRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const workspaceRoot = join(packageRoot, "../..");
const packageRequire = createRequire(import.meta.url);
const dshRoot = dirname(packageRequire.resolve("@deepseek-ai/dsh/package.json"));
const dshBinary = join(packageRoot, "node_modules", ".bin", "dsh");
const clientModulesRoot = dirname(
  packageRequire.resolve("@deepseek-ai/dsh-client-modules/package.json"),
);

async function packLocalPackage(destination: string): Promise<string> {
  execFileSync(
    "pnpm",
    ["--filter", "@tta-lab/dsh-web", "pack", "--pack-destination", destination],
    { cwd: workspaceRoot, stdio: "ignore" },
  );
  const files = await readdir(destination);
  const tarball = files.find((file) => file.endsWith(".tgz"));
  if (tarball === undefined) throw new Error("local dsh-web pack produced no tarball");
  return join(destination, tarball);
}

function testEnvironment(
  home: string,
  runnerWorkspace: string,
  recordPath: string,
  argsPath: string,
) {
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    DSH_HOME: home,
    VITEST: "true",
    ORGANON_PI_TEST_WORKSPACE: runnerWorkspace,
    PHASE4_DSH_RECORD: recordPath,
    PHASE4_DSH_ARGS: argsPath,
    DSH_TELEMETRY_DISABLED: "1",
  };
  for (const key of ["EXA_API_KEY", "BRAVE_API_KEY", "DEEPSEEK_API_KEY"]) delete env[key];
  return env;
}

async function writeProbePackage(profile: string): Promise<void> {
  const packageDir = join(profile, "node_modules", "phase4-dsh-probe");
  await mkdir(packageDir, { recursive: true });
  await writeFile(
    join(packageDir, "package.json"),
    JSON.stringify({
      name: "phase4-dsh-probe",
      version: "0.0.0",
      type: "module",
      main: "index.js",
    }) + "\n",
  );
  await writeFile(
    join(packageDir, "index.js"),
    [
      'import { writeFile } from "node:fs/promises";',
      'export const name = "phase4-dsh-probe";',
      'export const inject = ["agentPresets", "tools"];',
      "export async function apply(ctx) {",
      "  const recordPath = process.env.PHASE4_DSH_RECORD;",
      "  try {",
      '    if (recordPath === undefined) throw new Error("PHASE4_DSH_RECORD missing");',
      '    const scope = await ctx.agentPresets.standingKeyFor("code");',
      '    const search = ctx.tools.get("web_search", scope);',
      '    const fetch = ctx.tools.get("web_fetch", scope);',
      '    const rootFetch = ctx.tools.get("web_fetch");',
      '    if (search === undefined) throw new Error("stock code preset did not register web_search");',
      '    const result = await search.execute({ queries: ["-flag-like query", "normal query"] }, { signal: new AbortController().signal });',
      "    await writeFile(recordPath, JSON.stringify({ scopeSearch: true, scopeFetch: fetch !== undefined, rootFetch: rootFetch !== undefined, result }));",
      "  } catch (error) {",
      "    if (recordPath !== undefined) await writeFile(recordPath, JSON.stringify({ error: String(error) }));",
      "    throw error;",
      "  }",
      "}",
    ].join("\n"),
  );
  await writeFile(
    join(profile, "cordis.patch.yml"),
    "- insert:\n    - id: phase4-dsh-probe\n      name: phase4-dsh-probe\n",
  );
}

async function writeFakeWebBinary(binary: string, argsPath: string): Promise<void> {
  await mkdir(dirname(binary), { recursive: true });
  await writeFile(
    binary,
    [
      "#!/usr/bin/env node",
      'import { appendFileSync } from "node:fs";',
      "const args = process.argv.slice(2);",
      'const query = args[args.indexOf("--") + 1];',
      "appendFileSync(process.env.PHASE4_DSH_ARGS, JSON.stringify({ args, key: process.env.BRAVE_API_KEY ?? null, exa: process.env.EXA_API_KEY ?? null }) + String.fromCharCode(10));",
      'process.stdout.write(JSON.stringify({ provider: args[args.indexOf("--provider") + 1], results: [{ link: "https://example.test/" + encodeURIComponent(query), title: "Organon", snippet: "fake", position: 1 }] }));',
    ].join("\n"),
    { mode: 0o755 },
  );
  await writeFile(argsPath, "");
}

async function waitForRecord(
  recordPath: string,
  child: ChildProcess,
  stderr: () => string,
): Promise<any> {
  const deadline = Date.now() + 45_000;
  while (Date.now() < deadline) {
    if (existsSync(recordPath)) {
      const text = (await readFile(recordPath, "utf8")).trim();
      if (text !== "") return JSON.parse(text);
    }
    if (child.exitCode !== null) {
      throw new Error(`dsh exited ${child.exitCode} before probe completed: ${stderr()}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`dsh probe timed out: ${stderr()}`);
}

async function stopProcess(child: ChildProcess | undefined): Promise<void> {
  if (child === undefined || child.exitCode !== null) return;
  child.kill("SIGTERM");
  await Promise.race([
    new Promise<void>((resolve) => child.once("exit", () => resolve())),
    new Promise<void>((resolve) => setTimeout(resolve, 5_000)),
  ]);
  if (child.exitCode === null) child.kill("SIGKILL");
}

async function loadWithRealClientModuleSystem(clientPath: string): Promise<void> {
  const previousWindow = (globalThis as any).window;
  const pendingQueue: Array<{
    id: string;
    factory: (require: (specifier: string) => unknown) => any;
  }> = [];
  const target: any = {
    mode: "queue",
    pendingQueue,
    load(registration: any) {
      pendingQueue.push(registration);
    },
  };
  (globalThis as any).window = { __ModuleLoader__: target };
  try {
    const bootstrapPath = join(clientModulesRoot, "lib", "client.js");
    await import(`${pathToFileURL(bootstrapPath).href}?bootstrap=${Date.now()}`);
    const bootstrapIndex = pendingQueue.findIndex(
      (entry) => entry.id === "@deepseek-ai/dsh-client-modules",
    );
    const registration = pendingQueue[bootstrapIndex];
    if (registration === undefined) throw new Error("real rc.8 client bootstrap did not register");
    pendingQueue.splice(bootstrapIndex, 1);
    const bootstrap = registration.factory(() => {
      throw new Error("client bootstrap requested an unexpected external");
    });
    const react = await import(
      pathToFileURL(join(packageRoot, "node_modules", "react", "index.js")).href
    );
    const system = bootstrap.createClientModuleSystem(
      target,
      { id: registration.id, exports: bootstrap },
      {
        boot: {
          rev: "phase4",
          entries: [
            { id: "@tta-lab/dsh-web", url: "/plugins/@tta-lab/dsh-web/client.js", rev: "phase4" },
          ],
        },
        staticModules: { react },
        loadBundle: async () => {
          await import(`${pathToFileURL(clientPath).href}?client=${Date.now()}`);
        },
      },
    );
    const loaded = await system.import("@tta-lab/dsh-web/client");
    expect(target.mode).toBe("live");
    expect(typeof loaded.apply).toBe("function");
    expect(loaded.inject).toEqual(["connection", "remote", "settingsScope", "slots"]);
  } finally {
    if (previousWindow === undefined) delete (globalThis as any).window;
    else (globalThis as any).window = previousWindow;
  }
}

describe.sequential("actual DSH rc.8 normal Web profile composition", () => {
  it("installs through dsh, boots the stock code/PTC preset, and loads the client factory", async () => {
    const home = await mkdtemp(join(tmpdir(), "dsh rc8 actual "));
    const packs = join(home, "packs");
    const runnerWorkspace = join(home, "runner workspace");
    const recordPath = join(home, "probe.json");
    const argsPath = join(home, "args.jsonl");
    const binary = join(runnerWorkspace, "packages", "native", "pi-web-darwin-arm64", "bin", "web");
    const env = testEnvironment(home, runnerWorkspace, recordPath, argsPath);
    let child: ChildProcess | undefined;
    let stderr = "";
    try {
      await mkdir(packs, { recursive: true });
      await writeFakeWebBinary(binary, argsPath);
      const tarball = await packLocalPackage(packs);
      execFileSync(dshBinary, ["plugin", "--profile", "web", "add", "--offline", tarball], {
        cwd: workspaceRoot,
        env,
        stdio: "ignore",
      });
      const profile = join(home, "profiles", "web");
      const profileManifest = JSON.parse(
        await readFile(join(profile, "package.json"), "utf8"),
      ) as any;
      expect(profileManifest.dsh.profile.bundles).toEqual([
        "@deepseek-ai/dsh-base",
        "@deepseek-ai/dsh-web-app",
        "@tta-lab/dsh-web",
      ]);
      await writeFile(join(home, "settings.yaml"), "organon-web:\n  provider: duckduckgo\n");
      await writeProbePackage(profile);

      const dump = execFileSync(dshBinary, ["--profile", "web", "--dump-config"], {
        cwd: workspaceRoot,
        env,
        encoding: "utf8",
      });
      expect(dump).toMatch(
        /- id: web\n  name: '@deepseek-ai\/dsh-web'\n  config:\n    searchProvider: organon-web-search/,
      );
      expect(dump).toMatch(/- id: tool-web[\s\S]*?disabled: true/);
      expect(dump).toContain("- id: organon-dsh-web");
      expect(dump).toContain("- id: phase4-dsh-probe");

      const codePreset = await readFile(
        join(dshRoot, "config", "agent-presets", "code", "agent.cordis.yml"),
        "utf8",
      );
      expect(codePreset).toMatch(
        /- id: tool-web\n  name: '@deepseek-ai\/dsh-tool-web'\n  config:\n    fetch: false/,
      );
      expect(codePreset).toMatch(/- id: tool-presentation[\s\S]*?mode: code/);

      child = spawn(dshBinary, ["--profile", "web", "--no-open", "--port", "0"], {
        cwd: workspaceRoot,
        env,
        stdio: ["ignore", "pipe", "pipe"],
      });
      child.stderr?.on("data", (chunk) => {
        stderr += String(chunk);
      });
      const probe = await waitForRecord(recordPath, child, () => stderr);
      expect(probe.scopeSearch).toBe(true);
      expect(probe.scopeFetch).toBe(false);
      expect(probe.rootFetch).toBe(false);
      expect(probe.result.sources).toHaveLength(2);
      expect(probe.result.sources.every((source: any) => source.title === "Organon")).toBe(true);

      const records = (await readFile(argsPath, "utf8"))
        .trim()
        .split("\n")
        .filter(Boolean)
        .map(
          (line) => JSON.parse(line) as { args: string[]; key: string | null; exa: string | null },
        );
      expect(records).toHaveLength(2);
      const byQuery = new Map(
        records.map((record) => [record.args[record.args.indexOf("--") + 1], record]),
      );
      for (const query of ["-flag-like query", "normal query"]) {
        expect(byQuery.get(query)).toEqual({
          args: ["search", "--provider", "duckduckgo", "--json", "--", query],
          key: null,
          exa: null,
        });
      }

      const installedClient = join(
        profile,
        "node_modules",
        "@tta-lab",
        "dsh-web",
        "dist",
        "client.js",
      );
      await loadWithRealClientModuleSystem(installedClient);
    } finally {
      await stopProcess(child);
      await rm(home, { recursive: true, force: true });
    }
  });
});
