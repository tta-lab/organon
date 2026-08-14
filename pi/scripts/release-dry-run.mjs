#!/usr/bin/env node
// Verifies release invariants without network access:
// - all sixteen manifests share one version matching the release tag
// - every main package pins its native optional dependencies to that exact version
// - no manifest references latest or an unmatched version
// - when a goreleaser dist dir is given, its metadata version matches the tag
//   and its artifacts cover the four tools on the three supported platforms
//
// Usage: node scripts/release-dry-run.mjs <x.y.z> [goreleaserDistDir]
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const version = process.argv[2];
const dist = process.argv[3];

if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version ?? "")) {
  console.error("usage: node scripts/release-dry-run.mjs <x.y.z> [goreleaserDistDir]");
  process.exit(2);
}

const read = (p) => JSON.parse(readFileSync(p, "utf8"));
const mainPackages = readdirSync(join(workspace, "packages")).filter((d) => d.startsWith("pi-"));
const nativePackages = readdirSync(join(workspace, "packages", "native"));
const errors = [];

for (const dir of [...mainPackages, ...nativePackages]) {
  const pkgDir = nativePackages.includes(dir)
    ? join("packages", "native", dir)
    : join("packages", dir);
  const manifest = read(join(workspace, pkgDir, "package.json"));
  if (manifest.version !== version) {
    errors.push(`${manifest.name}: version ${manifest.version} != ${version}`);
  }
  if (JSON.stringify(manifest).includes("latest")) {
    errors.push(`${manifest.name}: manifest mentions latest`);
  }
}

for (const dir of mainPackages) {
  const manifest = read(join(workspace, "packages", dir, "package.json"));
  for (const [dep, spec] of Object.entries(manifest.optionalDependencies ?? {})) {
    if (spec !== version) {
      errors.push(`${manifest.name}: optional dep ${dep}@${spec} != ${version}`);
    }
    const nativeName = dep.replace("@tta-lab/pi-", "pi-");
    if (!nativePackages.includes(nativeName)) {
      errors.push(`${manifest.name}: optional dep ${dep} is not a workspace native package`);
    }
  }
}

// Tag -> GoReleaser artifact mapping: the dist metadata version (tag without
// the leading v) must equal the manifest version, and the binaries for the
// four tools on the three supported platforms must all be present.
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
  for (const tool of ["src", "web", "project", "og"]) {
    for (const [os, arch] of [
      ["darwin", "arm64"],
      ["linux", "amd64"],
      ["linux", "arm64"],
    ]) {
      const found = list.some(
        (a) => a.type === "Binary" && a.name === tool && a.goos === os && a.goarch === arch,
      );
      if (!found) {
        errors.push(`goreleaser artifacts missing ${tool}_${os}_${arch}`);
      }
    }
  }
}

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}
console.log(
  `dry-run ok: ${mainPackages.length + nativePackages.length} manifests at ${version}` +
    (dist ? `, goreleaser artifacts match ${version}` : ""),
);
