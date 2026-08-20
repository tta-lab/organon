#!/usr/bin/env node
// Stages GoReleaser-built binaries into the native npm package bin directories
// and verifies each staged binary matches its package's platform.
//
// Usage: node scripts/stage-natives.mjs <goreleaserDistDir>
// Every tool/os/arch combination must resolve to an exact per-platform
// artifact; a missing or cross-platform fallback is an error so a binary can
// never be shipped inside the wrong native package.
import { copyFileSync, chmodSync, mkdirSync, existsSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { artifactMatchesTool, NATIVE_TARGETS } from "./release-targets.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const workspace = join(here, "..");
const dist = process.argv[2];
if (!dist || !existsSync(dist)) {
  console.error("usage: node scripts/stage-natives.mjs <goreleaser-dist-dir>");
  process.exit(2);
}

/**
 * Reads goreleaser's artifacts.json for the exact per-build binary paths;
 * never resolves a same-named binary from another platform.
 */
function artifactFor(tool, target) {
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
      (a) =>
        a.type === "Binary" &&
        artifactMatchesTool(a.name, tool) &&
        a.goos === target.goos &&
        a.goarch === target.goarch,
    );
    if (match?.path) {
      // artifacts.json paths are relative to the directory containing dist/.
      const candidate = join(dirname(dist), match.path);
      if (existsSync(candidate)) {
        return candidate;
      }
    }
  }
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
for (const target of NATIVE_TARGETS) {
  for (const tool of target.tools) {
    const source = artifactFor(tool, target);
    if (!source) {
      console.error(
        `missing exact artifact for ${tool}_${target.goos}_${target.goarch}; refusing to fall back`,
      );
      process.exit(1);
    }
    const destDir = join(
      workspace,
      "packages",
      "native",
      `pi-${tool}-${target.packageSuffix}`,
      "bin",
    );
    mkdirSync(destDir, { recursive: true });
    const executable = target.goos === "windows" ? `${tool}.exe` : tool;
    const dest = join(destDir, executable);
    copyFileSync(source, dest);
    chmodSync(dest, 0o755);
    const signature = fileSignature(dest);
    if (!target.fileMarker.test(signature)) {
      console.error(
        `staged ${tool} for ${target.goos}/${target.goarch} but file reports: ${signature || "unreadable"}`,
      );
      process.exit(1);
    }
    console.log(`staged ${tool} -> ${dest} (${signature.split(",")[0]})`);
    staged++;
  }
}
console.log(`staged and verified ${staged} native binaries`);
