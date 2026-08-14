#!/usr/bin/env node
// Defines and consumes the single npm publish order for a Pi extension release.
// Native packages must be published before main packages because main packages
// declare them as exact-version optional dependencies.
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { join, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const defaultWorkspace = join(here, "..");

function packageEntries(workspace, relative, kind) {
  const directory = join(workspace, relative);
  return readdirSync(directory)
    .filter((entry) => entry.startsWith("pi-"))
    .sort()
    .map((dir) => {
      const path = join(directory, dir);
      const manifest = JSON.parse(readFileSync(join(path, "package.json"), "utf8"));
      return { kind, dir, name: manifest.name, path };
    });
}

// packagePublishPlan is the one source of the release publish order. The
// release workflow executes it through publishReleasePackages, while dry-run
// verification and tests inspect this same plan.
export function packagePublishPlan(workspace = defaultWorkspace) {
  return [
    ...packageEntries(workspace, join("packages", "native"), "native"),
    ...packageEntries(workspace, "packages", "main"),
  ];
}

export function assertNativePackagesFirst(plan) {
  let sawMain = false;
  for (const entry of plan) {
    if (entry.kind === "main") {
      sawMain = true;
      continue;
    }
    if (entry.kind !== "native") {
      throw new Error(`unknown publish package kind: ${entry.kind}`);
    }
    if (sawMain) {
      throw new Error("native packages must be published before main packages");
    }
  }
}

export function publishReleasePackages(plan = packagePublishPlan()) {
  assertNativePackagesFirst(plan);
  for (const entry of plan) {
    console.log(`publishing ${entry.name}`);
    execFileSync("pnpm", ["publish", "--no-git-checks", "--access", "public"], {
      cwd: entry.path,
      stdio: "inherit",
    });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  publishReleasePackages();
}
