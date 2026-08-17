import { chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterAll, describe, expect, it } from "vitest";

import {
  assertNativePackagesFirst,
  distTagForVersion,
  NPM_REGISTRY,
  packagePublishPlan,
  publishReleasePackages,
} from "../../../scripts/publish-packages.mjs";

const tmp = mkdtempSync(join(tmpdir(), "organon-publish-plan-"));

afterAll(() => rmSync(tmp, { recursive: true, force: true }));

function fakeNpm(mode: "missing" | "ambiguous" | "malformed", existing?: string): string {
  const path = join(tmp, `fake-npm-${mode}-${existing ? "existing" : "all"}.mjs`);
  writeFileSync(
    path,
    `#!/usr/bin/env node
import { appendFileSync } from "node:fs";
const args = process.argv.slice(2);
appendFileSync(process.env.FAKE_NPM_LOG, JSON.stringify({ args, cwd: process.cwd() }) + "\\n");
if (args[0] === "view") {
  const specifier = args[1];
  if (process.env.FAKE_NPM_MODE === "ambiguous") {
    console.error("npm error code E401");
    process.exit(1);
  }
  if (process.env.FAKE_NPM_MODE === "malformed") {
    process.stdout.write("not-json");
    process.exit(0);
  }
  if (specifier === process.env.FAKE_NPM_EXISTING) {
    process.stdout.write(JSON.stringify(specifier.slice(specifier.lastIndexOf("@") + 1)));
    process.exit(0);
  }
  console.error("npm error code E404");
  process.exit(1);
}
if (args[0] === "publish") process.exit(0);
console.error("unexpected fake npm command");
process.exit(2);
`,
  );
  chmodSync(path, 0o755);
  return path;
}

function isolatedEnv(log: string, mode: "missing" | "ambiguous" | "malformed", existing?: string) {
  const home = join(tmp, `home-${mode}-${existing ? "existing" : "all"}`);
  const cache = join(tmp, `cache-${mode}-${existing ? "existing" : "all"}`);
  const userConfig = join(tmp, `npmrc-${mode}-${existing ? "existing" : "all"}`);
  const globalConfig = join(tmp, `global-npmrc-${mode}-${existing ? "existing" : "all"}`);
  writeFileSync(userConfig, "");
  writeFileSync(globalConfig, "");
  return {
    PATH: process.env.PATH ?? "",
    HOME: home,
    NPM_CONFIG_USERCONFIG: userConfig,
    NPM_CONFIG_GLOBALCONFIG: globalConfig,
    NPM_CONFIG_CACHE: cache,
    NPM_CONFIG_OFFLINE: "true",
    NPM_CONFIG_IGNORE_SCRIPTS: "true",
    NPM_CONFIG_AUDIT: "false",
    NPM_CONFIG_FUND: "false",
    FAKE_NPM_LOG: log,
    FAKE_NPM_MODE: mode,
    ...(existing ? { FAKE_NPM_EXISTING: existing } : {}),
  };
}

function records(log: string): Array<{ args: string[]; cwd: string }> {
  return readFileSync(log, "utf8")
    .trim()
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line));
}

describe("release publish plan", () => {
  it("uses the actual release plan to publish every native package before any main package", () => {
    const plan = packagePublishPlan();

    expect(plan).toHaveLength(16);
    expect(plan.slice(0, 12).every((entry) => entry.kind === "native")).toBe(true);
    expect(plan.slice(12).every((entry) => entry.kind === "main")).toBe(true);
    expect(() => assertNativePackagesFirst([...plan].reverse())).toThrow(
      "native packages must be published before main packages",
    );
  });

  it("selects beta for prereleases and latest for stable versions", () => {
    expect(distTagForVersion("2.2.0-beta.1")).toBe("beta");
    expect(distTagForVersion("2.2.0")).toBe("latest");
  });

  it("passes the beta dist-tag to npm for a prerelease plan", () => {
    const source = packagePublishPlan()[0]!;
    const betaPath = join(tmp, "beta-package");
    mkdirSync(betaPath, { recursive: true });
    const manifest = JSON.parse(readFileSync(join(source.path, "package.json"), "utf8"));
    manifest.version = "2.2.0-beta.1";
    writeFileSync(join(betaPath, "package.json"), JSON.stringify(manifest, null, 2) + "\n");
    const log = join(tmp, "beta.log");
    const npmCommand = fakeNpm("missing");

    const result = publishReleasePackages(
      [{ ...source, path: betaPath, version: "2.2.0-beta.1" }],
      { npmCommand, env: isolatedEnv(log, "missing") },
    );

    expect(result).toEqual({
      version: "2.2.0-beta.1",
      distTag: "beta",
      published: [source.name],
      skipped: [],
    });
    expect(records(log).find((call) => call.args[0] === "publish")!.args).toContain("beta");
  });

  it("queries exact versions, skips existing packages, and publishes missing packages with npm", () => {
    const plan = packagePublishPlan();
    const log = join(tmp, "publish.log");
    const existing = plan[0]!;
    const npmCommand = fakeNpm("missing", existing.name + "@" + existing.version);
    const result = publishReleasePackages(plan, {
      npmCommand,
      env: isolatedEnv(log, "missing", existing.name + "@" + existing.version),
    });
    const calls = records(log);
    const views = calls.filter((call) => call.args[0] === "view");
    const publishes = calls.filter((call) => call.args[0] === "publish");

    expect(result).toEqual({
      version: "0.1.0",
      distTag: "latest",
      published: plan.slice(1).map((entry) => entry.name),
      skipped: [existing.name],
    });
    expect(views).toHaveLength(16);
    expect(publishes).toHaveLength(15);
    expect(views[0]!.args).toEqual([
      "view",
      `${existing.name}@${existing.version}`,
      "version",
      "--json",
      "--registry",
      NPM_REGISTRY,
    ]);
    expect(publishes[0]!.args).toEqual([
      "publish",
      "--no-git-checks",
      "--access",
      "public",
      "--tag",
      "latest",
      "--registry",
      NPM_REGISTRY,
    ]);
    expect(publishes.map((call) => call.cwd)).toEqual(plan.slice(1).map((entry) => entry.path));
  });

  it("fails closed on an ambiguous registry failure before publishing anything", () => {
    const plan = packagePublishPlan();
    const log = join(tmp, "ambiguous.log");
    const npmCommand = fakeNpm("ambiguous");

    expect(() =>
      publishReleasePackages(plan, {
        npmCommand,
        env: isolatedEnv(log, "ambiguous"),
      }),
    ).toThrow("registry lookup failed");
    expect(records(log)).toHaveLength(1);
  });

  it("fails closed on malformed registry metadata", () => {
    const plan = packagePublishPlan();
    const log = join(tmp, "malformed.log");
    const npmCommand = fakeNpm("malformed");

    expect(() =>
      publishReleasePackages(plan, {
        npmCommand,
        env: isolatedEnv(log, "malformed"),
      }),
    ).toThrow("malformed metadata");
    expect(records(log)).toHaveLength(1);
  });
});
