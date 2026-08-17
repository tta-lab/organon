#!/usr/bin/env node
// Defines and consumes the single npm publish order for a Pi extension release.
// Native packages must be published before main packages because main packages
// declare them as exact-version optional dependencies.
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const defaultWorkspace = join(here, "..");
export const NPM_REGISTRY = "https://registry.npmjs.org";

// This is intentionally local instead of importing a package manager's private
// semver implementation: the release plan only needs to classify a complete
// package version as stable or prerelease.
const SEMVER_RE =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

function readManifest(path) {
  try {
    return JSON.parse(readFileSync(join(path, "package.json"), "utf8"));
  } catch (error) {
    throw new Error(`invalid package metadata at ${join(path, "package.json")}: ${error.message}`);
  }
}

function packageEntries(workspace, relative, kind) {
  const directory = join(workspace, relative);
  return readdirSync(directory)
    .filter((entry) => entry.startsWith("pi-"))
    .sort()
    .flatMap((dir) => {
      const path = join(directory, dir);
      const manifest = readManifest(path);
      // Keep private workspace helpers out of the public release inventory if
      // one is ever placed under a pi-* directory.
      if (manifest.private === true) return [];
      if (typeof manifest.name !== "string" || !manifest.name) {
        throw new Error(`package metadata at ${join(path, "package.json")} has no name`);
      }
      if (typeof manifest.version !== "string" || !manifest.version) {
        throw new Error(`${manifest.name}: package metadata has no version`);
      }
      return [{ kind, dir, name: manifest.name, version: manifest.version, path }];
    });
}

// packagePublishPlan is the one source of the release publish order. The
// release workflow executes it through publishReleasePackages, while dry-run
// verification, version sync, and tests inspect this same plan.
export function packagePublishPlan(workspace = defaultWorkspace) {
  const plan = [
    ...packageEntries(workspace, join("packages", "native"), "native"),
    ...packageEntries(workspace, "packages", "main"),
  ];
  const names = new Set();
  for (const entry of plan) {
    if (names.has(entry.name))
      throw new Error(`duplicate public package in publish plan: ${entry.name}`);
    names.add(entry.name);
  }
  return plan;
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

export function distTagForVersion(version) {
  if (typeof version !== "string" || !SEMVER_RE.test(version)) {
    throw new Error(`invalid synchronized package version: ${version ?? ""}`);
  }
  return version.includes("-") ? "beta" : "latest";
}

function errorOutput(error) {
  return [error?.stderr, error?.stdout, error?.message]
    .map((value) => (value == null ? "" : String(value).trim()))
    .filter(Boolean)
    .join("\n");
}

function isUnambiguousNotFound(error) {
  const stderr = [error?.stderr, error?.stdout]
    .map((value) => (value == null ? "" : String(value)))
    .join("\n");
  return (
    /\bE404\b/i.test(stderr) ||
    /npm\s+error\s+404\b/i.test(stderr) ||
    /\b404\s+(?:not found|no match|does not exist)\b/i.test(stderr) ||
    /\bHTTP\/\d(?:\.\d)?\s+404\b/i.test(stderr)
  );
}

function validatePlanEntry(entry) {
  if (!entry || (entry.kind !== "native" && entry.kind !== "main")) {
    throw new Error(`invalid publish plan entry kind: ${entry?.kind ?? "missing"}`);
  }
  if (typeof entry.name !== "string" || !entry.name) {
    throw new Error("invalid publish plan entry name");
  }
  if (typeof entry.version !== "string" || !entry.version) {
    throw new Error(`${entry.name}: publish plan entry has no version`);
  }
  if (typeof entry.path !== "string" || !entry.path) {
    throw new Error(`${entry.name}: publish plan entry has no package path`);
  }

  const manifest = readManifest(entry.path);
  if (manifest.private === true)
    throw new Error(`${entry.name}: private packages cannot be published`);
  if (manifest.name !== entry.name) {
    throw new Error(
      `${entry.name}: publish plan name does not match package metadata ${manifest.name}`,
    );
  }
  if (manifest.version !== entry.version) {
    throw new Error(
      `${entry.name}: publish plan version ${entry.version} does not match metadata ${manifest.version}`,
    );
  }
  distTagForVersion(entry.version);
  return entry;
}

function readPublishedVersion(entry, options) {
  const { npmCommand, registry, env } = options;
  const specifier = `${entry.name}@${entry.version}`;
  let stdout;
  try {
    stdout = execFileSync(
      npmCommand,
      ["view", specifier, "version", "--json", "--registry", registry],
      { cwd: entry.path, env, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] },
    );
  } catch (error) {
    if (isUnambiguousNotFound(error)) return false;
    throw new Error(`registry lookup failed for ${specifier}: ${errorOutput(error)}`);
  }

  let observed;
  try {
    observed = JSON.parse(String(stdout).trim());
  } catch (error) {
    throw new Error(
      `registry lookup returned malformed metadata for ${specifier}: ${error.message}`,
    );
  }
  if (observed !== entry.version) {
    throw new Error(
      `registry lookup returned ${JSON.stringify(observed)} for ${specifier}; refusing to publish`,
    );
  }
  return true;
}

export function publishReleasePackages(
  plan = packagePublishPlan(),
  { npmCommand = "npm", registry = NPM_REGISTRY, env = process.env } = {},
) {
  if (!Array.isArray(plan) || plan.length === 0) throw new Error("publish plan is empty");
  assertNativePackagesFirst(plan);
  const entries = plan.map(validatePlanEntry);
  const version = entries[0].version;
  if (entries.some((entry) => entry.version !== version)) {
    throw new Error("all public packages must share one synchronized version");
  }
  const tag = distTagForVersion(version);
  const published = [];
  const skipped = [];

  for (const entry of entries) {
    const specifier = `${entry.name}@${entry.version}`;
    if (readPublishedVersion(entry, { npmCommand, registry, env })) {
      console.log(`skipping ${specifier}: exact version already exists`);
      skipped.push(entry.name);
      continue;
    }

    console.log(`publishing ${specifier} with dist-tag ${tag}`);
    execFileSync(
      npmCommand,
      ["publish", "--no-git-checks", "--access", "public", "--tag", tag, "--registry", registry],
      { cwd: entry.path, env, stdio: "inherit" },
    );
    published.push(entry.name);
  }

  console.log(
    `publish plan complete: ${published.length} published, ${skipped.length} skipped, ${version} (${tag})`,
  );
  return { version, distTag: tag, published, skipped };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    publishReleasePackages();
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}
