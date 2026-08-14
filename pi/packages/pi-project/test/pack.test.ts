import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterAll, describe, expect, it } from "vitest";

const workspace = join(import.meta.dirname, "..", "..", "..");
const pnpm = "pnpm";
const tmp = mkdtempSync(join(tmpdir(), "pi-project-pack-"));

const MAIN = "@tta-lab/pi-project";
const NATIVES = [
  "@tta-lab/pi-project-darwin-arm64",
  "@tta-lab/pi-project-linux-x64",
  "@tta-lab/pi-project-linux-arm64",
];

function pack(pkgDir: string, name: string): string {
  execFileSync(pnpm, ["pack", "--pack-destination", tmp], {
    cwd: pkgDir,
    stdio: "pipe",
  });
  const file = readdirSync(tmp).find((f) => f.includes(name.replace("@", "").replace("/", "-")))!;
  return join(tmp, file);
}

afterAll(() => {
  // keep artifacts in the tmp dir; CI cleans /tmp
});

describe("package manifests and packing", () => {
  it("packs the main package with exact-version optional native dependencies", () => {
    const tarball = pack(join(workspace, "packages", "pi-project"), MAIN);
    expect(existsSync(tarball)).toBe(true);
    // The tarball embeds package/package.json; pnpm pack rewrites workspace:* to exact versions.
    const out = execFileSync("tar", ["-xOf", tarball, "package/package.json"], {
      encoding: "utf8",
    });
    const manifest = JSON.parse(out);
    expect(manifest.name).toBe(MAIN);
    expect(manifest.version).toBe("0.1.0");
    expect(manifest.pi.extensions).toEqual(["./dist/index.js"]);
    for (const native of NATIVES) {
      expect(manifest.optionalDependencies[native]).toBe("0.1.0");
      expect(manifest.optionalDependencies[native]).not.toMatch(/[\^~]/);
      expect(manifest.optionalDependencies[native]).not.toMatch(/workspace/);
    }
    expect(Object.keys(manifest.optionalDependencies).sort()).toEqual([...NATIVES].sort());
  });

  it("packs each native package with os/cpu constraints and a bin entry", () => {
    const expected = {
      "@tta-lab/pi-project-darwin-arm64": { os: "darwin", cpu: "arm64" },
      "@tta-lab/pi-project-linux-x64": { os: "linux", cpu: "x64" },
      "@tta-lab/pi-project-linux-arm64": { os: "linux", cpu: "arm64" },
    };
    for (const [name, { os, cpu }] of Object.entries(expected)) {
      const pkgDir = join(
        workspace,
        "packages",
        "native",
        name.replace("@tta-lab/pi-project-", "pi-project-"),
      );
      const tarball = pack(pkgDir, name);
      const out = execFileSync("tar", ["-xOf", tarball, "package/package.json"], {
        encoding: "utf8",
      });
      const manifest = JSON.parse(out);
      expect(manifest.name).toBe(name);
      expect(manifest.version).toBe("0.1.0");
      expect(manifest.os).toEqual([os]);
      expect(manifest.cpu).toEqual([cpu]);
      expect(manifest.bin).toEqual({ project: "bin/project" });
    }
  });
});
