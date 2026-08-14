import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import {
  cliError,
  nativePackageName,
  parseSingleJsonDoc,
  resolveBinaryPath,
  runCli,
} from "../src/index.js";
import { detectPlatform } from "../src/platform.js";

const here = dirname(fileURLToPath(import.meta.url));
const fixtures = join(here, "fixtures");

describe("platform detection", () => {
  it("detects the current host as one of the supported triples", () => {
    const triple = detectPlatform();
    expect(["darwin", "linux"]).toContain(triple.os);
    expect(["arm64", "x64"]).toContain(triple.arch);
  });

  it("names the host native package for a tool", () => {
    const { os, arch } = detectPlatform();
    expect(nativePackageName("project")).toBe(`@tta-lab/pi-project-${os}-${arch}`);
  });
});

describe("binary resolution", () => {
  it("resolves the host native package's bin/<tool> via the resolver", () => {
    const { os, arch } = detectPlatform();
    const resolved = resolveBinaryPath("project", (specifier) => {
      expect(specifier).toBe(`@tta-lab/pi-project-${os}-${arch}/package.json`);
      return join(here, "fake-node-modules", specifier);
    });
    expect(resolved.endsWith(`/bin/project`) || resolved.endsWith(`\\bin\\project`)).toBe(true);
  });

  it("throws an actionable error when the native package is missing", () => {
    expect(() =>
      resolveBinaryPath("project", () => {
        throw new Error("Cannot find module");
      }),
    ).toThrow(/native package @tta-lab\/pi-project-/);
  });
});

describe("subprocess adapter", () => {
  it("passes fixed argv, writes stdin, and returns stdout/stderr/exitCode", async () => {
    const result = await runCli(process.execPath, {
      args: [join(fixtures, "echo-args.mjs"), "list", "--json"],
      stdin: "hello\nworld",
    });
    expect(result.exitCode).toBe(0);
    expect(result.stderr).toBe("diag\n");
    const out = JSON.parse(result.stdout);
    expect(out.argv).toEqual(["list", "--json"]);
    expect(out.stdin).toBe("hello\nworld");
  });

  it("rejects with Operation aborted when the signal fires", async () => {
    const controller = new AbortController();
    controller.abort();
    await expect(
      runCli(process.execPath, { args: [join(fixtures, "hang.mjs")], signal: controller.signal }),
    ).rejects.toThrow("Operation aborted");
  });

  it("propagates a nonzero exit code without throwing", async () => {
    const result = await runCli(process.execPath, { args: [join(fixtures, "fail.mjs")] });
    expect(result.exitCode).toBe(3);
    expect(result.stderr).toContain("boom");
  });

  it("rejects with a concise start failure for a missing binary", async () => {
    await expect(runCli(join(here, "does-not-exist"), { args: [] })).rejects.toThrow(
      /failed to start/,
    );
  });
});

describe("JSON result parsing", () => {
  it("parses exactly one JSON document and tolerates trailing newline", () => {
    expect(parseSingleJsonDoc<{ a: number }>('{"a":1}\n')).toEqual({ a: 1 });
  });

  it("rejects empty and invalid output", () => {
    expect(() => parseSingleJsonDoc("")).toThrow(/no JSON output/);
    expect(() => parseSingleJsonDoc('{"a":1} trailing')).toThrow(/invalid JSON/);
  });

  it("normalizes cobra errors and empty stderr", () => {
    expect(cliError("Error: text not found\n", 1).message).toBe("text not found");
    expect(cliError("", 7).message).toBe("command exited with code 7");
  });
});
