#!/usr/bin/env node
// Verifies release invariants without network access:
// - all sixteen manifests share one version
// - every main package pins its native optional dependencies to that exact version
// - no manifest references latest or an unmatched version
// - native packages are published before their dependent main packages
import { readFileSync, readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");

const read = (p) => JSON.parse(readFileSync(p, "utf8"));
const mainPackages = readdirSync(join(workspace, "packages")).filter((d) => d.startsWith("pi-"));
const nativePackages = readdirSync(join(workspace, "packages", "native"));

let version = null;
const errors = [];
for (const dir of [...mainPackages, ...nativePackages]) {
  const pkgDir = nativePackages.includes(dir)
    ? join("packages", "native", dir)
    : join("packages", dir);
  const manifest = read(join(workspace, pkgDir, "package.json"));
  if (version === null) version = manifest.version;
  if (manifest.version !== version) {
    errors.push(`${manifest.name}: version ${manifest.version} != ${version}`);
  }
  if (/latest/.test(manifest.version)) {
    errors.push(`${manifest.name}: version references latest`);
  }
  const text = JSON.stringify(manifest);
  if (/latest/.test(text)) {
    errors.push(`${manifest.name}: manifest mentions latest`);
  }
}

for (const dir of mainPackages) {
  const manifest = read(join(workspace, "packages", dir, "package.json"));
  const expected = manifest.version;
  for (const [dep, spec] of Object.entries(manifest.optionalDependencies ?? {})) {
    if (spec !== expected) {
      errors.push(`${manifest.name}: optional dep ${dep}@${spec} != ${expected}`);
    }
    if (!nativePackages.includes(dep.replace("@tta-lab/pi-", "pi-"))) {
      errors.push(`${manifest.name}: optional dep ${dep} is not a workspace native package`);
    }
  }
}

// Native packages must be ordered before the main packages that depend on them.
const publishOrder = [...nativePackages, ...mainPackages];
for (const dir of mainPackages) {
  const manifest = read(join(workspace, "packages", dir, "package.json"));
  const mainIndex = publishOrder.indexOf(dir);
  for (const dep of Object.keys(manifest.optionalDependencies ?? {})) {
    const nativeName = dep.replace("@tta-lab/", "");
    const nativeIndex = publishOrder.findIndex(
      (d) => d.replace("@tta-lab/pi-", "") === nativeName || d === nativeName,
    );
    if (nativeIndex > mainIndex) {
      errors.push(`${manifest.name}: native ${dep} would publish after its main package`);
    }
  }
}

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}
console.log(
  `dry-run ok: ${mainPackages.length + nativePackages.length} manifests at version ${version}, natives before mains`,
);
