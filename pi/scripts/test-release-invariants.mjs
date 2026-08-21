#!/usr/bin/env node
// CI-friendly release invariant test: copies the single publish plan into a
// disposable workspace, syncs it to a throwaway version (or the goreleaser
// snapshot version when a dist dir is given), and validates the invariants
// including the tag-to-artifact mapping without mutating the development tree.
//
// Usage: node scripts/test-release-invariants.mjs [goreleaserDistDir]
import { execFileSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { packagePublishPlan } from "./publish-packages.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const requestedDist = process.argv[2];
const dist = requestedDist === undefined ? undefined : resolve(workspace, requestedDist);
const sourcePlan = packagePublishPlan(workspace);

function createFixtureWorkspace(plan) {
  const fixture = mkdtempSync(join(tmpdir(), "organon-release-invariants-"));
  mkdirSync(join(fixture, "packages", "native"), { recursive: true });
  mkdirSync(join(fixture, "scripts"), { recursive: true });
  for (const entry of plan) {
    const destination = join(fixture, relative(workspace, entry.path));
    mkdirSync(destination, { recursive: true });
    copyFileSync(join(entry.path, "package.json"), join(destination, "package.json"));
  }
  for (const script of [
    "publish-packages.mjs",
    "release-dry-run.mjs",
    "release-targets.mjs",
    "sync-version.mjs",
  ]) {
    copyFileSync(join(workspace, "scripts", script), join(fixture, "scripts", script));
  }
  return fixture;
}

let testVersion = "0.0.0-test";
if (dist && existsSync(join(dist, "metadata.json"))) {
  const meta = JSON.parse(readFileSync(join(dist, "metadata.json"), "utf8"));
  testVersion = String(meta.version ?? testVersion).replace(/^v/, "");
}

const fixture = createFixtureWorkspace(sourcePlan);
try {
  const fixturePlan = packagePublishPlan(fixture);
  if (fixturePlan.length !== sourcePlan.length) {
    throw new Error(
      `release fixture package count ${fixturePlan.length} != source plan ${sourcePlan.length}`,
    );
  }
  execFileSync(process.execPath, ["scripts/sync-version.mjs", testVersion], {
    cwd: fixture,
    stdio: "inherit",
  });
  const args = ["scripts/release-dry-run.mjs", testVersion];
  if (dist) args.push(dist);
  execFileSync(process.execPath, args, { cwd: fixture, stdio: "inherit" });
  console.log(
    `release invariants hold for ${fixturePlan.length} manifests at ${testVersion}` +
      (dist ? " with goreleaser artifacts" : ""),
  );
} finally {
  rmSync(fixture, { recursive: true, force: true });
  console.log("test-owned release fixture removed");
}
