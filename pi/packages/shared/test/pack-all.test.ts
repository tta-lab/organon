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
  });

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

  it("installs every packed main package offline and executes its tool through the Pi entry", async () => {
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
      const installRoot = join(workspace, ".tmp", `pack-install-${tool}`);
      rmSync(installRoot, { recursive: true, force: true });
      mkdirSync(installRoot, { recursive: true });
      const tgz = pack(join(workspace, "packages", `pi-${tool}`), `@tta-lab/pi-${tool}`);
      execFileSync("tar", ["-xzf", tgz, "-C", installRoot], { stdio: "pipe" });
      const pkg = join(installRoot, "package");
      expect(existsSync(join(pkg, "dist", "index.js"))).toBe(true);

      const nativePkgDir = join(
        pkg,
        "node_modules",
        "@tta-lab",
        `pi-${tool}-${osName}-${archName}`,
      );
      const nativeDir = join(nativePkgDir, "bin");
      mkdirSync(nativeDir, { recursive: true });
      writeFileSync(
        join(nativePkgDir, "package.json"),
        JSON.stringify({ name: `@tta-lab/pi-${tool}-${osName}-${archName}`, version: "0.1.0" }),
      );
      copyFileSync(join(workspace, fixture), join(nativeDir, tool));
      chmodSync(join(nativeDir, tool), 0o755);

      const registered: any[] = [];
      const fakePi = {
        registerTool: (d: any) => registered.push(d),
        on: () => undefined,
      };
      const mod = (await import("file://" + join(pkg, "dist", "index.js"))) as {
        default: (pi: unknown) => void;
      };
      mod.default(fakePi as any);
      expect(registered).toHaveLength(1);
      expect(registered[0]!.name).toBe(tool);

      const result = await registered[0]!.execute("id", action, undefined, undefined, {
        cwd: tmp,
      });
      assert(result.details);
    }
  });
});
