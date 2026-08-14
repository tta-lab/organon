#!/usr/bin/env node
// Stages GoReleaser-built binaries into the native npm package bin directories
// and verifies each staged binary matches its package's platform.
//
// Usage: node scripts/stage-natives.mjs <goreleaserDistDir>
// Every tool/os/arch combination must resolve to an exact per-platform
// artifact; a missing or cross-platform fallback is an error so a Darwin
// binary can never be shipped inside a Linux package.
import { copyFileSync, chmodSync, mkdirSync, existsSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
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
  ["darwin", "arm64", "darwin-arm64", /Mach-O.*arm64/],
  ["linux", "amd64", "linux-x64", /ELF.*x86-64/],
  ["linux", "arm64", "linux-arm64", /ELF.*aarch64/],
];

/**
 * Reads goreleaser's artifacts.json for the exact per-build binary paths;
 * falls back to globbing version-suffixed build directories when the metadata
 * file is absent. Never resolves a same-named binary from another platform.
 */
function artifactFor(tool, os, arch) {
  const metaPath = join(dist, "artifacts.json");
  if (existsSync(metaPath)) {
    let artifacts;
    try {
      artifacts = JSON.parse(readFileSync(metaPath, "utf8"));
    } catch {
      artifacts = [];
    }
    const list = Array.isArray(artifacts) ? artifacts : (artifacts.artifacts ?? []);
    const match = list.find(
      (a) => a.type === "Binary" && a.name === tool && a.goos === os && a.goarch === arch,
    );
    if (match?.path) {
      // artifacts.json paths are relative to the directory containing dist/.
      const candidate = join(dirname(dist), match.path);
      if (existsSync(candidate)) {
        return candidate;
      }
    }
  }
  // Fallback: dist/<tool>_<os>_<arch>_<version-suffix>/<tool>
  return undefined;
}

function fileSignature(binary) {
  try {
    return execFileSync("file", [binary], { encoding: "utf8" }).trim();
  } catch {
    return "";
  }
}

let staged = 0;
for (const tool of TOOLS) {
  for (const [os, arch, suffix, marker] of TARGETS) {
    const source = artifactFor(tool, os, arch);
    if (!source) {
      console.error(`missing exact artifact for ${tool}_${os}_${arch}; refusing to fall back`);
      process.exit(1);
    }
    const destDir = join(workspace, "packages", "native", `pi-${tool}-${suffix}`, "bin");
    mkdirSync(destDir, { recursive: true });
    const dest = join(destDir, tool);
    copyFileSync(source, dest);
    chmodSync(dest, 0o755);
    const signature = fileSignature(dest);
    if (!marker.test(signature)) {
      console.error(
        `staged ${tool} for ${os}/${arch} but file reports: ${signature || "unreadable"}`,
      );
      process.exit(1);
    }
    console.log(`staged ${tool} -> ${dest} (${signature.split(",")[0]})`);
    staged++;
  }
}
console.log(`staged and verified ${staged} native binaries`);
