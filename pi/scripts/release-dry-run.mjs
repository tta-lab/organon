#!/usr/bin/env node
// Verifies release invariants without network access:
// - all discovered public manifests share one version matching the release tag
// - every main package pins its native optional dependencies to that exact version
// - native packages are all referenced by a main package and no manifest references latest
// - when a goreleaser dist dir is given, its metadata and configured artifacts match
//
// Usage: node scripts/release-dry-run.mjs <x.y.z> [goreleaserDistDir]
import { readFileSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { artifactMatchesTool, NATIVE_TARGETS } from "./release-targets.mjs";
import {
  assertNativePackagesFirst,
  distTagForVersion,
  packagePublishPlan,
} from "./publish-packages.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const version = process.argv[2];
const dist = process.argv[3];

try {
  distTagForVersion(version);
} catch {
  console.error("usage: node scripts/release-dry-run.mjs <x.y.z> [goreleaserDistDir]");
  process.exit(2);
}

const read = (p) => JSON.parse(readFileSync(p, "utf8"));
const publishPlan = packagePublishPlan(workspace);
const mainPackages = publishPlan.filter((entry) => entry.kind === "main");
const nativePackages = publishPlan.filter((entry) => entry.kind === "native");
const nativePackageDirs = new Set(nativePackages.map((entry) => entry.dir));
const referencedNativeDirs = new Set();
const errors = [];
for (const entry of mainPackages) {
  const manifest = read(join(entry.path, "package.json"));
  for (const dep of Object.keys(manifest.optionalDependencies ?? {})) {
    referencedNativeDirs.add(dep.replace("@tta-lab/pi-", "pi-"));
  }
}
for (const dir of referencedNativeDirs) {
  if (!nativePackageDirs.has(dir)) {
    errors.push(`main package references missing native package ${dir}`);
  }
}
for (const dir of nativePackageDirs) {
  if (!referencedNativeDirs.has(dir)) {
    errors.push(`native package ${dir} is not referenced by a main package`);
  }
}
try {
  assertNativePackagesFirst(publishPlan);
} catch (error) {
  errors.push(error.message);
}

for (const entry of publishPlan) {
  const manifest = read(join(entry.path, "package.json"));
  if (manifest.version !== version) {
    errors.push(`${manifest.name}: version ${manifest.version} != ${version}`);
  }
  if (JSON.stringify(manifest).includes("latest")) {
    errors.push(`${manifest.name}: manifest mentions latest`);
  }
}

for (const entry of mainPackages) {
  const manifest = read(join(entry.path, "package.json"));
  for (const [dep, spec] of Object.entries(manifest.optionalDependencies ?? {})) {
    if (spec !== version) {
      errors.push(`${manifest.name}: optional dep ${dep}@${spec} != ${version}`);
    }
    const nativeName = dep.replace("@tta-lab/pi-", "pi-");
    if (!nativePackageDirs.has(nativeName)) {
      errors.push(`${manifest.name}: optional dep ${dep} is not a workspace native package`);
    }
  }
}

// Tag -> GoReleaser artifact mapping: the dist metadata version (tag without
// the leading v) must equal the manifest version, and every configured tool/
// platform target must be present.
if (dist) {
  const metaPath = join(dist, "metadata.json");
  if (!existsSync(metaPath)) {
    errors.push(`goreleaser metadata missing at ${metaPath}`);
  } else {
    const meta = read(metaPath);
    const metaVersion = String(meta.version ?? "").replace(/^v/, "");
    if (metaVersion !== version) {
      errors.push(`goreleaser metadata version ${metaVersion} != tag version ${version}`);
    }
    // The tag field is the release tag goreleaser ran under; snapshot runs
    // carry the previous tag, so only compare it for real (non-snapshot)
    // releases where version == tag without the leading v.
    if (!metaVersion.includes("-SNAPSHOT-") && meta.tag && meta.tag.replace(/^v/, "") !== version) {
      errors.push(`goreleaser tag ${meta.tag} != tag version ${version}`);
    }
  }
  const artifactsPath = join(dist, "artifacts.json");
  const artifacts = existsSync(artifactsPath) ? read(artifactsPath) : [];
  const list = Array.isArray(artifacts) ? artifacts : (artifacts.artifacts ?? []);
  for (const target of NATIVE_TARGETS) {
    for (const tool of target.tools) {
      const found = list.some(
        (a) =>
          a.type === "Binary" &&
          artifactMatchesTool(a.name, tool) &&
          a.goos === target.goos &&
          a.goarch === target.goarch,
      );
      if (!found) {
        errors.push(`goreleaser artifacts missing ${tool}_${target.goos}_${target.goarch}`);
      }
    }
  }
}

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}
console.log(
  `dry-run ok: ${publishPlan.length} manifests at ${version}` +
    (dist ? `, goreleaser artifacts match ${version}` : ""),
);
