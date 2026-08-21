#!/usr/bin/env node
// Syncs one release version across the single data-driven publish plan and
// pins main-package optionalDependencies to that exact version.
//
// Usage: node scripts/sync-version.mjs <version> [--dry-run]
//   version: npm version without the leading v (for example 1.2.3)
import { readFileSync, writeFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { distTagForVersion, packagePublishPlan } from "./publish-packages.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const version = process.argv[2];
const dryRun = process.argv.includes("--dry-run");

try {
  distTagForVersion(version);
} catch {
  console.error("usage: node scripts/sync-version.mjs <x.y.z> [--dry-run]");
  process.exit(2);
}

const plan = packagePublishPlan(workspace);
for (const entry of plan) {
  const manifestPath = join(entry.path, "package.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  const before = JSON.stringify(manifest, null, 2);
  manifest.version = version;
  if (entry.kind === "main" && manifest.optionalDependencies) {
    for (const key of Object.keys(manifest.optionalDependencies)) {
      manifest.optionalDependencies[key] = version;
    }
  }
  const after = JSON.stringify(manifest, null, 2);
  if (before !== after) {
    console.log(`${dryRun ? "[dry-run] would sync " : "synced "}${manifest.name} -> ${version}`);
    if (!dryRun) writeFileSync(manifestPath, after + "\n");
  }
}
console.log(`${dryRun ? "[dry-run] " : ""}${plan.length} manifests match version ${version}`);
