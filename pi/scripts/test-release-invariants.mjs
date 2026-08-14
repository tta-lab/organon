#!/usr/bin/env node
// CI-friendly release invariant test: syncs all sixteen manifests to a
// throwaway version, validates the invariants, then restores the workspace
// manifests so the dev tree keeps workspace:* optional dependencies.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const TEST_VERSION = "0.0.0-test";

const mainPackages = readdirSync(join(workspace, "packages")).filter((d) => d.startsWith("pi-"));
const nativePackages = readdirSync(join(workspace, "packages", "native"));

const manifests = [...mainPackages, ...nativePackages];
const original = new Map();
for (const dir of manifests) {
  const pkgDir = nativePackages.includes(dir)
    ? join("packages", "native", dir)
    : join("packages", dir);
  const path = join(workspace, pkgDir, "package.json");
  original.set(path, readFileSync(path, "utf8"));
}

try {
  execFileSync(process.execPath, ["scripts/sync-version.mjs", TEST_VERSION], {
    cwd: workspace,
    stdio: "inherit",
  });
  execFileSync(process.execPath, ["scripts/release-dry-run.mjs"], {
    cwd: workspace,
    stdio: "inherit",
  });
  console.log(`release invariants hold for ${manifests.length} manifests`);
} finally {
  for (const [path, content] of original) {
    writeFileSync(path, content);
  }
  console.log("workspace manifests restored");
}
