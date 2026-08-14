#!/usr/bin/env node
// Syncs one release version across all sixteen npm package manifests and pins
// main-package optionalDependencies to that exact version.
//
// Usage: node scripts/sync-version.mjs <version> [--dry-run]
//   version: npm version without the leading v (for example 1.2.3)
import { readFileSync, writeFileSync, readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const version = process.argv[2];
const dryRun = process.argv.includes("--dry-run");

if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version ?? "")) {
  console.error("usage: node scripts/sync-version.mjs <x.y.z> [--dry-run]");
  process.exit(2);
}

const mainPackages = readdirSync(join(workspace, "packages")).filter((d) => d.startsWith("pi-"));
const nativePackages = readdirSync(join(workspace, "packages", "native"));

for (const dir of [...mainPackages, ...nativePackages]) {
  const pkgDir = nativePackages.includes(dir)
    ? join("packages", "native", dir)
    : join("packages", dir);
  const manifestPath = join(workspace, pkgDir, "package.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  const before = JSON.stringify(manifest, null, 2);
  manifest.version = version;
  if (mainPackages.includes(dir) && manifest.optionalDependencies) {
    for (const key of Object.keys(manifest.optionalDependencies)) {
      manifest.optionalDependencies[key] = version;
    }
  }
  const after = JSON.stringify(manifest, null, 2);
  if (before !== after) {
    console.log(`${dryRun ? "[dry-run] would sync " : "synced "}${manifest.name} -> ${version}`);
    if (!dryRun) {
      writeFileSync(manifestPath, after + "\n");
    }
  }
}
console.log(`${dryRun ? "[dry-run] " : ""}16 manifests match version ${version}`);
