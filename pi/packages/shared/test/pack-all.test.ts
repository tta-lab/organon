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
const TOOLS = ["src", "web", "project", "og"];
const TARGETS = [
  ["darwin", "arm64", "darwin-arm64"],
  ["linux", "x64", "linux-x64"],
  ["linux", "arm64", "linux-arm64"],
];
const tmp = mkdtempSync(join(tmpdir(), "pi-pack-all-"));
const readManifest = (tgz: string) =>
  JSON.parse(execFileSync("tar", ["-xOf", tgz, "package/package.json"], { encoding: "utf8" }));

function pack(dir: string, name: string): string {
  execFileSync("pnpm", ["pack", "--pack-destination", tmp], { cwd: dir, stdio: "pipe" });
  const file = readdirSync(tmp).find((f) => f.includes(name.replace("@", "").replace("/", "-")))!;
  return join(tmp, file);
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
      action: unknown;
      fixture: string;
      assert: (details: any) => void;
    }> = [
      {
        tool: "src",
        action: { action: "symbols", path: join(tmp, "smoke.go") },
        fixture: "packages/pi-src/test/fixtures/bin/src",
        assert: (details: any) => expect(details.symbols[0]!.name).toBe("Foo"),
      },
      {
        tool: "web",
        action: { action: "search", query: "tree-sitter" },
        fixture: "packages/pi-web/test/fixtures/bin/web",
        assert: (details: any) => expect(details.provider).toBe("DuckDuckGo"),
      },
      {
        tool: "project",
        action: { action: "list" },
        fixture: "packages/pi-project/test/fixtures/bin/project",
        assert: (details: any) => expect(details.projects.length).toBeGreaterThan(0),
      },
      {
        tool: "og",
        action: { action: "auth_status", project: "ko" },
        fixture: "packages/pi-og/test/fixtures/bin/og",
        assert: (details: any) => expect(details.auth.ready).toBe(true),
      },
    ];

    writeFileSync(join(tmp, "smoke.go"), "package sample\n\nfunc Foo() {}\n");
    for (const { tool, action, fixture, assert } of smoke) {
      // Pack the native package from a throwaway copy so tests never write
      // fixtures into the workspace native packages (a leftover fixture would
      // be picked up by local debugging as the tool binary).
      const hostSuffix = `${osName}-${archName}`;
      const nativePkgName = `@tta-lab/pi-${tool}-${hostSuffix}`;
      const nativePkgDir = join(workspace, "packages", "native", `pi-${tool}-${hostSuffix}`);
      const manifest = JSON.parse(readFileSync(join(nativePkgDir, "package.json"), "utf8"));
      const tempNative = join(tmp, `native-${tool}-${hostSuffix}`);
      rmSync(tempNative, { recursive: true, force: true });
      mkdirSync(join(tempNative, "bin"), { recursive: true });
      writeFileSync(join(tempNative, "package.json"), JSON.stringify(manifest, null, 2));
      copyFileSync(join(workspace, fixture), join(tempNative, "bin", tool));
      chmodSync(join(tempNative, "bin", tool), 0o755);

      const nativeTgz = pack(tempNative, nativePkgName);
      const mainTgz = pack(join(workspace, "packages", `pi-${tool}`), `@tta-lab/pi-${tool}`);

      const installRoot = join(tmp, `install-${tool}`);
      rmSync(installRoot, { recursive: true, force: true });
      mkdirSync(installRoot, { recursive: true });
      execFileSync("tar", ["-xzf", mainTgz, "-C", installRoot], { stdio: "pipe" });
      const pkg = join(installRoot, "package");
      expect(existsSync(join(pkg, "dist", "index.js"))).toBe(true);

      // Install the packed native tarball exactly where npm would place the
      // matching optional dependency.
      const installedNative = join(pkg, "node_modules", "@tta-lab", `pi-${tool}-${hostSuffix}`);
      mkdirSync(installedNative, { recursive: true });
      execFileSync("tar", ["-xzf", nativeTgz, "-C", installedNative, "--strip-components=1"], {
        stdio: "pipe",
      });
      expect(existsSync(join(installedNative, "bin", tool))).toBe(true);

      // Real Pi discovery: the package manager resolve path feeds the loader.
      const { discoverAndLoadExtensions } = await import("@earendil-works/pi-coding-agent");
      const agentDir = join(tmp, `agent-${tool}`);
      mkdirSync(agentDir, { recursive: true });
      const loaded = await discoverAndLoadExtensions([pkg], tmp, agentDir);
      expect(loaded.errors).toEqual([]);
      const extension = loaded.extensions.find((e) => e.tools.has(tool));
      expect(extension).toBeDefined();
      const registered = extension!.tools.get(tool)!;
      expect(registered.definition.name).toBe(tool);

      const result = await registered.definition.execute("id", action, undefined, undefined, {
        cwd: tmp,
      } as any);
      assert(result.details);
    }
  }, 180000);
});
