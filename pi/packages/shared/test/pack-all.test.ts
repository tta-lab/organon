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

import { NATIVE_TOOLS, nativeTargetsForTool } from "../../../scripts/release-targets.mjs";

const workspace = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const testWorkspace = process.env.ORGANON_PI_TEST_WORKSPACE;
const TOOLS = NATIVE_TOOLS;
const tmp = mkdtempSync(join(tmpdir(), "pi-pack-all-"));
const hostileNpmUserConfig = join(tmp, "hostile-npmrc");
writeFileSync(hostileNpmUserConfig, "omit=optional\nignore-scripts=false\n");
const hostileNpmEnv: NodeJS.ProcessEnv = {
  PATH: process.env.PATH ?? "",
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
const packageFiles = (tgz: string) =>
  execFileSync("tar", ["-tzf", tgz], { encoding: "utf8" }).split("\n").filter(Boolean);

function expectPublicMetadata(manifest: any, directory: string): void {
  expect(manifest.homepage).toBe("https://github.com/tta-lab/organon#readme");
  expect(manifest.bugs).toEqual({ url: "https://github.com/tta-lab/organon/issues" });
  expect(manifest.repository).toEqual({
    type: "git",
    url: "git+https://github.com/tta-lab/organon.git",
    directory,
  });
  expect(manifest.license).toBe("Apache-2.0");
  expect(manifest.publishConfig).toEqual({
    registry: "https://registry.npmjs.org",
    access: "public",
  });
}

function hostSuffix(): string {
  const osName =
    process.platform === "darwin"
      ? "darwin"
      : process.platform === "linux"
        ? "linux"
        : process.platform === "win32"
          ? "win32"
          : "";
  if (!osName || !["arm64", "x64"].includes(process.arch)) {
    throw new Error(`unsupported pack host ${process.platform}-${process.arch}`);
  }
  if (osName === "darwin" && process.arch !== "arm64") {
    throw new Error(`unsupported pack host ${process.platform}-${process.arch}`);
  }
  if (osName === "win32" && process.arch !== "x64") {
    throw new Error(`unsupported pack host ${process.platform}-${process.arch}`);
  }
  return `${osName}-${process.arch}`;
}

function nativePackageDir(tool: string, suffix: string): string {
  const root = suffix === hostSuffix() ? (testWorkspace ?? workspace) : workspace;
  return join(root, "packages", "native", `pi-${tool}-${suffix}`);
}

let packCount = 0;
function pack(dir: string, name: string): string {
  // Pack into a fresh per-call directory so stale tarballs from earlier calls
  // (or earlier runs) can never be selected by name.
  const dest = join(tmp, `pack-${packCount++}`);
  mkdirSync(dest, { recursive: true });
  execFileSync("pnpm", ["pack", "--pack-destination", dest], {
    cwd: dir,
    stdio: "pipe",
    env: offlineNpmEnv,
  });
  const files = readdirSync(dest).filter((f) => f.endsWith(".tgz"));
  if (files.length !== 1) {
    throw new Error(`expected one tarball in ${dest}, got ${files.join(",")}`);
  }
  return join(dest, files[0]!);
}

afterAll(() => {
  // keep artifacts in the tmp dir for inspection; CI cleans /tmp
});

describe("all native package manifests", () => {
  it("packs every native package with os/cpu constraints, a bin entry, and one version", () => {
    const version = readManifest(
      pack(join(workspace, "packages", "pi-src"), "@tta-lab/pi-src"),
    ).version;
    for (const tool of TOOLS) {
      for (const target of nativeTargetsForTool(tool)) {
        const dir = join(workspace, "packages", "native", `pi-${tool}-${target.packageSuffix}`);
        const manifest = readManifest(pack(dir, `@tta-lab/pi-${tool}-${target.packageSuffix}`));
        expect(manifest.version).toBe(version);
        expectPublicMetadata(manifest, `pi/packages/native/pi-${tool}-${target.packageSuffix}`);
        expect(manifest.os).toEqual([target.packageOS]);
        expect(manifest.cpu).toEqual([target.packageCPU]);
        const executable = target.goos === "windows" ? `${tool}.exe` : tool;
        expect(manifest.bin).toEqual({ [tool]: `bin/${executable}` });
      }
    }
  }, 120000);

  it("packs every main package with public metadata, a README, and exact-version optional native dependencies", () => {
    const version = readManifest(
      pack(join(workspace, "packages", "pi-src"), "@tta-lab/pi-src"),
    ).version;
    for (const tool of TOOLS) {
      const tgz = pack(join(workspace, "packages", `pi-${tool}`), `@tta-lab/pi-${tool}`);
      const manifest = readManifest(tgz);
      expect(manifest.version).toBe(version);
      expectPublicMetadata(manifest, `pi/packages/pi-${tool}`);
      expect(packageFiles(tgz)).toContain("package/README.md");
      expect(manifest.pi.extensions).toEqual(["./dist/index.js"]);
      for (const { packageSuffix: suffix } of nativeTargetsForTool(tool)) {
        const dep = `@tta-lab/pi-${tool}-${suffix}`;
        expect(manifest.optionalDependencies[dep]).toBe(version);
        expect(manifest.optionalDependencies[dep]).not.toMatch(/[\^~]/);
      }
      expect(Object.keys(manifest.optionalDependencies).length).toBe(
        nativeTargetsForTool(tool).length,
      );
    }
  });

  it("installs packed main and native tarballs offline and executes the src read/edit overrides", async () => {
    const currentHostSuffix = hostSuffix();

    const smoke: Array<{
      tool: string;
      publicTool?: string;
      action: unknown;
      assert: (result: any) => void;
    }> = [
      {
        tool: "src",
        publicTool: "read",
        action: { path: join(tmp, "smoke.go") },
        assert: (result: any) => expect(result.content[0].text).toContain("func Foo"),
      },
      {
        tool: "src",
        publicTool: "edit",
        action: {
          path: join(tmp, "smoke.go"),
          edits: [{ oldText: "func Foo() {}", newText: "func Foo() { return 1 }" }],
        },
        assert: (result: any) => {
          expect(result.details.diff).toContain("return 1");
          expect(result.details.patch).toContain("--- a/");
        },
      },
      {
        tool: "web",
        publicTool: "web_search",
        action: { queries: ["tree-sitter"] },
        assert: (result: any) => expect(result.details.provider).toBe("DuckDuckGo"),
      },
      {
        tool: "project",
        publicTool: "project_list",
        action: {},
        assert: (result: any) => expect(result.details.projects.length).toBeGreaterThan(0),
      },
      {
        tool: "og",
        publicTool: "og_push",
        action: { project: "ko" },
        assert: (result: any) => expect(result.details.message).toContain("push completed"),
      },
    ];

    writeFileSync(join(tmp, "smoke.go"), "package sample\n\nfunc Foo() {}\n");
    const smokeTools =
      currentHostSuffix === "win32-x64" ? smoke.filter(({ tool }) => tool === "web") : smoke;
    for (const { tool, publicTool = tool, action, assert } of smokeTools) {
      const nativePkgName = `@tta-lab/pi-${tool}-${currentHostSuffix}`;
      const nativePkgDir = nativePackageDir(tool, currentHostSuffix);
      const nativeTgz = pack(nativePkgDir, nativePkgName);
      const mainTgz = pack(join(workspace, "packages", `pi-${tool}`), `@tta-lab/pi-${tool}`);

      const installRoot = join(tmp, `install-${tool}-${publicTool}`);
      rmSync(installRoot, { recursive: true, force: true });
      mkdirSync(installRoot, { recursive: true });
      const overrides: Record<string, string> = {};
      for (const { packageSuffix: suffix } of nativeTargetsForTool(tool)) {
        const platformDir = nativePackageDir(tool, suffix);
        const tgz = pack(platformDir, `@tta-lab/pi-${tool}-${suffix}`);
        overrides[`@tta-lab/pi-${tool}-${suffix}`] = `file:${tgz}`;
      }
      writeFileSync(
        join(installRoot, "package.json"),
        JSON.stringify({ name: "smoke", private: true, overrides }),
      );
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
      const installedNative = join(installRoot, "node_modules", nativePkgName);
      const installedExecutable = currentHostSuffix === "win32-x64" ? `${tool}.exe` : tool;
      expect(existsSync(join(installedNative, "bin", installedExecutable))).toBe(true);
      for (const { packageSuffix: suffix } of nativeTargetsForTool(tool)) {
        if (suffix !== currentHostSuffix) {
          expect(
            existsSync(join(installRoot, "node_modules", "@tta-lab", `pi-${tool}-${suffix}`)),
          ).toBe(false);
        }
      }

      const { discoverAndLoadExtensions } = await import("@earendil-works/pi-coding-agent");
      const agentDir = join(tmp, `agent-${tool}-${publicTool}`);
      mkdirSync(agentDir, { recursive: true });
      const loaded = await discoverAndLoadExtensions([pkg], tmp, agentDir);
      expect(loaded.errors).toEqual([]);
      const extension = loaded.extensions.find((entry) => entry.tools.has(publicTool));
      expect(extension).toBeDefined();
      const registered = extension!.tools.get(publicTool)!;
      expect(registered.definition.name).toBe(publicTool);

      const result = await registered.definition.execute("id", action, undefined, undefined, {
        cwd: tmp,
      } as any);
      assert(result);
    }
  }, 180000);
});
