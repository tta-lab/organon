#!/usr/bin/env node
// Stages GoReleaser-built binaries into the native npm package bin directories.
//
// Usage: node scripts/stage-natives.mjs <goreleaserDistDir>
// The dist dir is expected to contain per-build outputs named like
//   src_darwin_arm64/src, web_linux_amd64/web, og_linux_arm64/og, ...
import { copyFileSync, chmodSync, mkdirSync, existsSync, readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const dist = process.argv[2];
if (!dist || !existsSync(dist)) {
  console.error("usage: node scripts/stage-natives.mjs <goreleaser-dist-dir>");
  process.exit(2);
}

const TOOLS = ["src", "web", "project", "og"];
const TARGETS = [
  ["darwin", "arm64", "darwin-arm64"],
  ["linux", "amd64", "linux-x64"],
  ["linux", "arm64", "linux-arm64"],
];

let staged = 0;
for (const tool of TOOLS) {
  for (const [os, arch, suffix] of TARGETS) {
    const candidates = [
      join(dist, `${tool}_${os}_${arch}`, tool),
      join(dist, `${os}_${arch}_${tool}`, tool),
    ];
    const source = candidates.find((c) => existsSync(c));
    if (!source) {
      // Search recursively as a fallback for goreleaser naming differences.
      const found = findBinary(dist, tool);
      if (!found) {
        console.error(`missing binary for ${tool}_${os}_${arch}`);
        process.exit(1);
      }
    }
    const destDir = join(workspace, "packages", "native", `pi-${tool}-${suffix}`, "bin");
    mkdirSync(destDir, { recursive: true });
    const dest = join(destDir, tool);
    copyFileSync(source ?? findBinary(dist, tool), dest);
    chmodSync(dest, 0o755);
    console.log(`staged ${tool} -> ${dest}`);
    staged++;
  }
}
console.log(`staged ${staged} native binaries`);

function findBinary(root, tool) {
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const full = join(root, entry.name);
    if (entry.isDirectory()) {
      const nested = findBinary(full, tool);
      if (nested) return nested;
    } else if (entry.name === tool) {
      return full;
    }
  }
  return undefined;
}
