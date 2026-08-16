#!/usr/bin/env node
// CI-friendly release invariant test: syncs the single publish plan to a
// throwaway version (or the goreleaser snapshot version when a dist dir is
// given), validates the invariants including the tag-to-artifact mapping, then
// restores the workspace manifests so the dev tree keeps workspace:*.
//
// Usage: node scripts/test-release-invariants.mjs [goreleaserDistDir]
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { packagePublishPlan } from "./publish-packages.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const dist = process.argv[2];
const plan = packagePublishPlan(workspace);

let testVersion = "0.0.0-test";
if (dist && existsSync(join(dist, "metadata.json"))) {
  const meta = JSON.parse(readFileSync(join(dist, "metadata.json"), "utf8"));
  testVersion = String(meta.version ?? testVersion).replace(/^v/, "");
}

const original = new Map();
for (const entry of plan) {
  const path = join(entry.path, "package.json");
  original.set(path, readFileSync(path, "utf8"));
}

try {
  execFileSync(process.execPath, ["scripts/sync-version.mjs", testVersion], {
    cwd: workspace,
    stdio: "inherit",
  });
  const args = ["scripts/release-dry-run.mjs", testVersion];
  if (dist) args.push(dist);
  execFileSync(process.execPath, args, { cwd: workspace, stdio: "inherit" });
  console.log(
    `release invariants hold for ${plan.length} manifests at ${testVersion}` +
      (dist ? " with goreleaser artifacts" : ""),
  );
} finally {
  for (const [path, content] of original) writeFileSync(path, content);
  console.log("workspace manifests restored");
}
