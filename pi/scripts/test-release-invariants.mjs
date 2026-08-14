#!/usr/bin/env node
// CI-friendly release invariant test: syncs all sixteen manifests to a
// throwaway version (or the goreleaser snapshot version when a dist dir is
// given), validates the invariants including the tag-to-artifact mapping, then
// restores the workspace manifests so the dev tree keeps workspace:*.
//
// Usage: node scripts/test-release-invariants.mjs [goreleaserDistDir]
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const dist = process.argv[2];

let testVersion = "0.0.0-test";
if (dist && existsSync(join(dist, "metadata.json"))) {
  const meta = JSON.parse(readFileSync(join(dist, "metadata.json"), "utf8"));
  testVersion = String(meta.version ?? testVersion).replace(/^v/, "");
}

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
  execFileSync(process.execPath, ["scripts/sync-version.mjs", testVersion], {
    cwd: workspace,
    stdio: "inherit",
  });
  const args = ["scripts/release-dry-run.mjs", testVersion];
  if (dist) {
    args.push(dist);
  }
  execFileSync(process.execPath, args, { cwd: workspace, stdio: "inherit" });
  console.log(
    `release invariants hold for ${manifests.length} manifests at ${testVersion}` +
      (dist ? " with goreleaser artifacts" : ""),
  );
} finally {
  for (const [path, content] of original) {
    writeFileSync(path, content);
  }
  console.log("workspace manifests restored");
}
