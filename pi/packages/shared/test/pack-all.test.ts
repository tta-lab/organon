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

  it("bundles the shared adapter into each main package (no fifth runtime package)", () => {
    for (const tool of TOOLS) {
      const dist = readFileSync(
        join(workspace, "packages", `pi-${tool}`, "dist", "index.js"),
        "utf8",
      );
      expect(dist).not.toContain("@tta-lab/pi-shared");
      expect(dist).toContain("resolveBinaryPath");
      expect(dist).toContain("spawn");
    }
  });

  it("installs a packed main package offline and executes its tool through the Pi entry", async () => {
    const installRoot = join(workspace, ".tmp", "pack-install");
    rmSync(installRoot, { recursive: true, force: true });
    mkdirSync(installRoot, { recursive: true });
    const tgz = pack(join(workspace, "packages", "pi-src"), "@tta-lab/pi-src");
    execFileSync("tar", ["-xzf", tgz, "-C", installRoot], { stdio: "pipe" });
    const pkg = join(installRoot, "package");
    expect(existsSync(join(pkg, "dist", "index.js"))).toBe(true);

    const { platform, arch } = process;
    const osName = platform === "darwin" ? "darwin" : "linux";
    const archName = arch === "arm64" ? "arm64" : "x64";
    const nativePkgDir = join(pkg, "node_modules", "@tta-lab", `pi-src-${osName}-${archName}`);
    const nativeDir = join(nativePkgDir, "bin");
    mkdirSync(nativeDir, { recursive: true });
    writeFileSync(
      join(nativePkgDir, "package.json"),
      JSON.stringify({ name: `@tta-lab/pi-src-${osName}-${archName}`, version: "0.1.0" }),
    );
    copyFileSync(
      join(workspace, "packages", "pi-src", "test", "fixtures", "bin", "src"),
      join(nativeDir, "src"),
    );
    chmodSync(join(nativeDir, "src"), 0o755);

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
    expect(registered[0]!.name).toBe("src");

    const tmpFile = join(tmp, "smoke.go");
    writeFileSync(tmpFile, "package sample\n\nfunc Foo() {}\n");
    const result = await registered[0]!.execute(
      "id",
      { action: "symbols", path: tmpFile },
      undefined,
      undefined,
      {
        cwd: tmp,
      },
    );
    const details = result.details as { symbols: Array<{ name: string }> };
    expect(details.symbols[0]!.name).toBe("Foo");
  });
});
