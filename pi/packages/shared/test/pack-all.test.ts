import { execFileSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { afterAll, describe, expect, it } from "vitest";

const workspace = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const testWorkspace = process.env.ORGANON_PI_TEST_WORKSPACE;
const TOOLS = ["src", "web", "project", "og"];
const TARGETS: Array<[string, string, string]> = [
  ["darwin", "arm64", "darwin-arm64"],
  ["linux", "x64", "linux-x64"],
  ["linux", "arm64", "linux-arm64"],
];
const tmp = mkdtempSync(join(tmpdir(), "pi-pack-all-"));
const hostileNpmUserConfig = join(tmp, "hostile-npmrc");
writeFileSync(hostileNpmUserConfig, "omit=optional\nignore-scripts=false\n");
const hostileNpmEnv = {
  ...process.env,
  NPM_CONFIG_USERCONFIG: hostileNpmUserConfig,
  NPM_CONFIG_OMIT: "optional",
  npm_config_registry: "https://registry.invalid",
};
const isolatedNpmUserConfig = join(tmp, "npmrc");
const isolatedNpmGlobalConfig = join(tmp, "npm-globalrc");
const isolatedNpmHome = join(tmp, "npm-home");
const isolatedNpmCache = join(tmp, "npm-cache");
writeFileSync(isolatedNpmUserConfig, "");
writeFileSync(isolatedNpmGlobalConfig, "");
mkdirSync(isolatedNpmHome, { recursive: true });
mkdirSync(isolatedNpmCache, { recursive: true });

function isolatedNpmEnv(base: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  const env = Object.fromEntries(
    Object.entries(base).filter(([key]) => !key.toLowerCase().startsWith("npm_config_")),
  );
  delete env.NODE_AUTH_TOKEN;
  delete env.NPM_TOKEN;
  return {
    ...env,
    HOME: isolatedNpmHome,
    NPM_CONFIG_USERCONFIG: isolatedNpmUserConfig,
    NPM_CONFIG_GLOBALCONFIG: isolatedNpmGlobalConfig,
    NPM_CONFIG_CACHE: isolatedNpmCache,
    NPM_CONFIG_OFFLINE: "true",
    NPM_CONFIG_LEGACY_PEER_DEPS: "true",
    NPM_CONFIG_IGNORE_SCRIPTS: "true",
    NPM_CONFIG_AUDIT: "false",
    NPM_CONFIG_FUND: "false",
  };
}

const offlineNpmEnv = isolatedNpmEnv(hostileNpmEnv);
const readManifest = (tgz: string) =>
  JSON.parse(execFileSync("tar", ["-xOf", tgz, "package/package.json"], { encoding: "utf8" }));

function nativePackageDir(tool: string, suffix: string): string {
  const hostSuffix = `${process.platform === "darwin" ? "darwin" : "linux"}-${process.arch}`;
  const root = suffix === hostSuffix ? (testWorkspace ?? workspace) : workspace;
  return join(root, "packages", "native", `pi-${tool}-${suffix}`);
}

let packCount = 0;
function pack(dir: string, name: string): string {
  // Pack into a fresh per-call directory so stale tarballs from earlier calls
  // (or earlier runs) can never be selected by name.
  const dest = join(tmp, `pack-${packCount++}`);
  mkdirSync(dest, { recursive: true });
  execFileSync("pnpm", ["pack", "--pack-destination", dest], { cwd: dir, stdio: "pipe" });
  const files = readdirSync(dest).filter((f) => f.endsWith(".tgz"));
  if (files.length !== 1) {
    throw new Error(`expected one tarball in ${dest}, got ${files.join(",")}`);
  }
  return join(dest, files[0]!);
}

afterAll(() => {
  // keep artifacts in the tmp dir for inspection; CI cleans /tmp
});

describe("all sixteen package manifests", () => {
  it("packs every native package with os/cpu constraints, a bin entry, and one version", () => {
    const version = readManifest(
      pack(join(workspace, "packages", "pi-src"), "@tta-lab/pi-src"),
    ).version;
    for (const tool of TOOLS) {
      for (const [os, cpu, suffix] of TARGETS) {
        const dir = join(workspace, "packages", "native", `pi-${tool}-${suffix}`);
        const manifest = readManifest(pack(dir, `@tta-lab/pi-${tool}-${suffix}`));
        expect(manifest.version).toBe(version);
        expect(manifest.os).toEqual([os]);
        expect(manifest.cpu).toEqual([cpu]);
        expect(manifest.bin).toEqual({ [tool]: `bin/${tool}` });
      }
    }
  }, 120000);

  it("packs every main package with exact-version optional native dependencies", () => {
    for (const tool of TOOLS) {
      const manifest = readManifest(
        pack(join(workspace, "packages", `pi-${tool}`), `@tta-lab/pi-${tool}`),
      );
      expect(manifest.version).toBe("0.1.0");
      expect(manifest.pi.extensions).toEqual(["./dist/index.js"]);
      for (const [, , suffix] of TARGETS) {
        const dep = `@tta-lab/pi-${tool}-${suffix}`;
        expect(manifest.optionalDependencies[dep]).toBe("0.1.0");
        expect(manifest.optionalDependencies[dep]).not.toMatch(/[\^~]/);
      }
      expect(Object.keys(manifest.optionalDependencies).length).toBe(3);
    }
  });

  it("installs packed main and native tarballs offline and discovers the extension through Pi's loader", async () => {
    const { platform, arch } = process;
    const osName = platform === "darwin" ? "darwin" : "linux";
    const archName = arch === "arm64" ? "arm64" : "x64";

    const smoke: Array<{
      tool: string;
      publicTool?: string;
      action: unknown;
      assert: (details: any) => void;
    }> = [
      {
        tool: "src",
        action: { action: "symbols", path: join(tmp, "smoke.go") },
        assert: (details: any) => expect(details.symbols[0]!.name).toBe("Foo"),
      },
      {
        tool: "web",
        action: { action: "search", query: "tree-sitter" },
        assert: (details: any) => expect(details.provider).toBe("DuckDuckGo"),
      },
      {
        tool: "project",
        action: { action: "list" },
        assert: (details: any) => expect(details.projects.length).toBeGreaterThan(0),
      },
      {
        tool: "og",
        publicTool: "og_auth_status",
        action: { project: "ko" },
        assert: (details: any) => expect(details.auth.ready).toBe(true),
      },
    ];

    writeFileSync(join(tmp, "smoke.go"), "package sample\n\nfunc Foo() {}\n");
    for (const { tool, publicTool = tool, action, assert } of smoke) {
      // Pack the host native package from Vitest's disposable fixture
      // workspace; repository native package directories remain untouched.
      const hostSuffix = `${osName}-${archName}`;
      const nativePkgName = `@tta-lab/pi-${tool}-${hostSuffix}`;
      const nativePkgDir = nativePackageDir(tool, hostSuffix);
      const nativeTgz = pack(nativePkgDir, nativePkgName);
      const mainTgz = pack(join(workspace, "packages", `pi-${tool}`), `@tta-lab/pi-${tool}`);

      // Real package-manager install: npm resolves the main tarball's optional
      // dependencies (pinned to the packed native tarballs via overrides) and
      // applies os/cpu selection. Peer dependencies are skipped because pi
      // bundles them; the offline flag keeps the registry out of the test.
      const installRoot = join(tmp, `install-${tool}`);
      rmSync(installRoot, { recursive: true, force: true });
      mkdirSync(installRoot, { recursive: true });
      const overrides: Record<string, string> = {};
      for (const [, , suffix] of TARGETS) {
        const platformDir = nativePackageDir(tool, suffix);
        const tgz = pack(platformDir, `@tta-lab/pi-${tool}-${suffix}`);
        overrides[`@tta-lab/pi-${tool}-${suffix}`] = `file:${tgz}`;
      }
      writeFileSync(
        join(installRoot, "package.json"),
        JSON.stringify({ name: "smoke", private: true, overrides }),
      );
      // Run npm from the project dir so the overrides in its package.json
      // apply; the main tarball is referenced by its absolute path.
      execFileSync(
        "npm",
        [
          "install",
          "--offline",
          "--legacy-peer-deps",
          "--ignore-scripts",
          "--no-audit",
          "--no-fund",
          mainTgz,
        ],
        { cwd: installRoot, stdio: "pipe", env: offlineNpmEnv },
      );

      const pkg = join(installRoot, "node_modules", `@tta-lab/pi-${tool}`);
      expect(existsSync(join(pkg, "dist", "index.js"))).toBe(true);
      // The host platform's native optional dependency was selected and
      // installed with its binary; the other platforms were skipped.
      const installedNative = join(installRoot, "node_modules", nativePkgName);
      expect(existsSync(join(installedNative, "bin", tool))).toBe(true);
      console.log("TOOL", tool, "bin ok");
      for (const [, , suffix] of TARGETS) {
        if (suffix !== hostSuffix) {
          expect(
            existsSync(join(installRoot, "node_modules", "@tta-lab", `pi-${tool}-${suffix}`)),
          ).toBe(false);
        }
      }

      // Real Pi discovery: the package manager resolve path feeds the loader.
      const { discoverAndLoadExtensions } = await import("@earendil-works/pi-coding-agent");
      const agentDir = join(tmp, `agent-${tool}`);
      mkdirSync(agentDir, { recursive: true });
      const loaded = await discoverAndLoadExtensions([pkg], tmp, agentDir);
      expect(loaded.errors).toEqual([]);
      const extension = loaded.extensions.find((e) => e.tools.has(publicTool));
      expect(extension).toBeDefined();
      const registered = extension!.tools.get(publicTool)!;
      expect(registered.definition.name).toBe(publicTool);

      const result = await registered.definition.execute("id", action, undefined, undefined, {
        cwd: tmp,
      } as any);
      assert(result.details);
    }
  }, 180000);
});
