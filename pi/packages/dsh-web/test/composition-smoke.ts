import { existsSync } from "node:fs";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { createRequire } from "node:module";
import { mkdir, mkdtemp, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { detectPlatform } from "@tta-lab/pi-shared";

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
  const path = join(destination, tarball);
  const packedFiles = execFileSync("tar", ["-tzf", path], { encoding: "utf8" })
    .split("\n")
    .filter(Boolean);
  expect(packedFiles).toEqual(
    expect.arrayContaining([
      "package/dist/index.js",
      "package/dist/index.d.ts",
      "package/dist/client.js",
      "package/dist/client.d.cts",
      "package/cordis.patch.yml",
      "package/README.md",
    ]),
  );
  const manifest = JSON.parse(
    execFileSync("tar", ["-xOf", path, "package/package.json"], { encoding: "utf8" }),
  );
  expect(manifest.main).toBe("dist/index.js");
  expect(manifest.exports["./client"]).toEqual({
    types: "./dist/client.d.cts",
    default: "./dist/client.js",
  });
  expect(Object.keys(manifest.optionalDependencies).sort()).toEqual(
    [
      "@tta-lab/pi-web-darwin-arm64",
      "@tta-lab/pi-web-linux-arm64",
      "@tta-lab/pi-web-linux-x64",
      "@tta-lab/pi-web-win32-x64",
    ].sort(),
  );
  expect(new Set(Object.values(manifest.optionalDependencies))).toEqual(
    new Set([manifest.version]),
  );
  return path;
}

async function packLocalNativePackage(
  destination: string,
  os: string,
  arch: string,
  executable: string,
  argsPath: string,
): Promise<string> {
  const packageDir = join(destination, `pi-web-${os}-${arch}`);
  await mkdir(join(packageDir, "bin"), { recursive: true });
  const sourceManifest = JSON.parse(
    await readFile(
      join(workspaceRoot, "packages", "native", `pi-web-${os}-${arch}`, "package.json"),
      "utf8",
    ),
  );
  await writeFile(join(packageDir, "package.json"), JSON.stringify(sourceManifest) + "\n");
  await writeFakeWebBinary(join(packageDir, "bin", executable), argsPath);
  const packDestination = join(destination, "tarball");
  await mkdir(packDestination, { recursive: true });
  execFileSync("pnpm", ["pack", "--pack-destination", packDestination], {
    cwd: packageDir,
    stdio: "ignore",
  });
  const files = (await readdir(packDestination)).filter((file) => file.endsWith(".tgz"));
  if (files.length !== 1)
    throw new Error("test-owned web native pack produced an invalid tarball set");
  return join(packDestination, files[0]!);
}

function testEnvironment(home: string, recordPath: string, argsPath: string) {
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    DSH_HOME: home,
    PHASE6_DSH_RECORD: recordPath,
    PHASE6_DSH_ARGS: argsPath,
    DSH_TELEMETRY_DISABLED: "1",
  };
  for (const key of [
    "EXA_API_KEY",
    "BRAVE_API_KEY",
    "CONTEXT7_API_KEY",
    "DEEPSEEK_API_KEY",
    "VITEST",
    "ORGANON_PI_TEST_WORKSPACE",
  ])
    delete env[key];
  return env;
}

async function writeProbePackage(profile: string): Promise<void> {
  const packageDir = join(profile, "node_modules", "phase6-dsh-probe");
  await mkdir(packageDir, { recursive: true });
  await writeFile(
    join(packageDir, "package.json"),
    JSON.stringify({
      name: "phase6-dsh-probe",
      version: "0.0.0",
      type: "module",
      main: "index.js",
    }) + "\n",
  );
  await writeFile(
    join(packageDir, "index.js"),
    [
      'import { writeFile } from "node:fs/promises";',
      'export const name = "phase6-dsh-probe";',
      'export const inject = ["agentPresets", "tools"];',
      "export async function apply(ctx) {",
      "  const recordPath = process.env.PHASE6_DSH_RECORD;",
      "  try {",
      '    if (recordPath === undefined) throw new Error("PHASE6_DSH_RECORD missing");',
      '    const scope = await ctx.agentPresets.standingKeyFor("code");',
      '    const search = ctx.tools.get("web_search", scope);',
      '    const globalSearch = ctx.tools.get("web_search");',
      '    const fetch = ctx.tools.get("web_fetch", scope);',
      '    const globalFetch = ctx.tools.get("web_fetch");',
      '    const docs = ctx.tools.get("web_docs", scope);',
      '    const sgraph = ctx.tools.get("web_sgraph", scope);',
      '    if (search === undefined) throw new Error("stock code preset did not register web_search");',
      '    if (globalSearch !== undefined) throw new Error("plugin replaced native web_search");',
      '    if (fetch === undefined || globalFetch === undefined) throw new Error("plugin fetch tool is missing");',
      '    if (docs === undefined || sgraph === undefined) throw new Error("plugin docs/source tools are missing");',
      '    const result = await search.execute({ queries: ["-flag-like query", "normal query"] }, { signal: new AbortController().signal });',
      "    await writeFile(recordPath, JSON.stringify({ scopeSearch: true, globalSearch: false, scopeFetch: fetch !== undefined, globalFetch: globalFetch !== undefined, docs: docs !== undefined, sgraph: sgraph !== undefined, fetchFields: Object.keys(fetch.parameters.properties ?? {}), result }));",
      "  } catch (error) {",
      "    if (recordPath !== undefined) await writeFile(recordPath, JSON.stringify({ error: String(error) }));",
      "    throw error;",
      "  }",
      "}",
    ].join("\n"),
  );
  await writeFile(
    join(profile, "cordis.patch.yml"),
    "- insert:\n    - id: phase6-dsh-probe\n      name: phase6-dsh-probe\n",
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
      "appendFileSync(process.env.PHASE6_DSH_ARGS, JSON.stringify({ args, key: process.env.BRAVE_API_KEY ?? null, exa: process.env.EXA_API_KEY ?? null }) + String.fromCharCode(10));",
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
          rev: "phase6",
          entries: [
            { id: "@tta-lab/dsh-web", url: "/plugins/@tta-lab/dsh-web/client.js", rev: "phase6" },
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
    // This executes the current host only. Windows web.exe execution is proven by
    // the Phase 3 Windows CI job; this smoke retains Windows package invariants.

    const home = await mkdtemp(join(tmpdir(), "dsh rc8 actual "));
    const packs = join(home, "packs");
    const recordPath = join(home, "probe.json");
    const argsPath = join(home, "args.jsonl");
    const { os, arch } = detectPlatform("web");
    const executable = os === "win32" ? "web.exe" : "web";
    const env = testEnvironment(home, recordPath, argsPath);
    let child: ChildProcess | undefined;
    let stderr = "";
    try {
      await mkdir(packs, { recursive: true });
      const tarball = await packLocalPackage(packs);
      const nativeTarball = await packLocalNativePackage(
        join(packs, "native"),
        os,
        arch,
        executable,
        argsPath,
      );
      execFileSync(
        dshBinary,
        ["plugin", "--profile", "web", "add", "--offline", tarball, nativeTarball],
        {
          cwd: workspaceRoot,
          env,
          stdio: "ignore",
        },
      );
      const profile = join(home, "profiles", "web");
      const profileManifest = JSON.parse(
        await readFile(join(profile, "package.json"), "utf8"),
      ) as any;
      expect(profileManifest.dsh.profile.bundles).toEqual([
        "@deepseek-ai/dsh-base",
        "@deepseek-ai/dsh-web-app",
        "@tta-lab/dsh-web",
      ]);
      const installedNative = join(profile, "node_modules", "@tta-lab", `pi-web-${os}-${arch}`);
      expect(existsSync(join(installedNative, "package.json"))).toBe(true);
      expect(existsSync(join(installedNative, "bin", executable))).toBe(true);
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
      expect(dump).toContain("- id: phase6-dsh-probe");

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
      if (probe.error !== undefined) throw new Error(`${probe.error}\n${stderr}`);
      expect(probe.scopeSearch).toBe(true);
      expect(probe.globalSearch).toBe(false);
      expect(probe.scopeFetch).toBe(true);
      expect(probe.globalFetch).toBe(true);
      expect(probe.docs).toBe(true);
      expect(probe.sgraph).toBe(true);
      expect(probe.fetchFields).toEqual(["url", "tree", "section_id", "full", "tree_threshold"]);
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
