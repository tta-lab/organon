import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
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
const fixtures = join(here, "..", "testdata");

async function waitForFile(path: string): Promise<void> {
  const deadline = Date.now() + 1000;
  while (Date.now() < deadline) {
    if (existsSync(path)) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error(`child did not write ${path}`);
}

async function waitForProcessExit(pid: number): Promise<void> {
  const deadline = Date.now() + 1000;
  while (Date.now() < deadline) {
    try {
      process.kill(pid, 0);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ESRCH") {
        return;
      }
      throw error;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error(`child ${pid} did not exit after cancellation`);
}

describe("platform detection", () => {
  it("detects the current host as one of the supported triples", () => {
    const triple = detectPlatform();
    expect(["darwin", "linux", "win32"]).toContain(triple.os);
    expect(["arm64", "x64"]).toContain(triple.arch);
  });

  it("names the host native package for a tool", () => {
    const { os, arch } = detectPlatform();
    const tool = os === "win32" ? "web" : "project";
    expect(nativePackageName(tool)).toBe(`@tta-lab/pi-${tool}-${os}-${arch}`);
  });
});

describe("binary resolution", () => {
  it("resolves the host native package's bin/<tool> via the resolver", () => {
    const { os, arch } = detectPlatform();
    const tool = os === "win32" ? "web" : "project";
    const resolved = resolveBinaryPath(tool, {
      resolve: (specifier) => {
        expect(specifier).toBe(`@tta-lab/pi-${tool}-${os}-${arch}/package.json`);
        return join(here, "fake-node-modules", specifier);
      },
    });
    const executable = os === "win32" ? `${tool}.exe` : tool;
    expect(resolved.replaceAll("\\", "/").endsWith(`/bin/${executable}`)).toBe(true);
  });

  it("throws an actionable error when the native package is missing", () => {
    const tool = detectPlatform().os === "win32" ? "web" : "project";
    expect(() =>
      resolveBinaryPath(tool, {
        resolve: () => {
          throw new Error("Cannot find module");
        },
      }),
    ).toThrow(new RegExp(`native package @tta-lab/pi-${tool}-`));
  });
});

function withPlatform<T>(platform: NodeJS.Platform, arch: NodeJS.Architecture, run: () => T): T {
  const originalPlatform = Object.getOwnPropertyDescriptor(process, "platform");
  const originalArch = Object.getOwnPropertyDescriptor(process, "arch");
  if (!originalPlatform || !originalArch)
    throw new Error("process platform descriptors unavailable");
  Object.defineProperty(process, "platform", { value: platform });
  Object.defineProperty(process, "arch", { value: arch });
  try {
    return run();
  } finally {
    Object.defineProperty(process, "platform", originalPlatform);
    Object.defineProperty(process, "arch", originalArch);
  }
}

describe("Windows web binary resolution", () => {
  it("resolves win32/x64 web.exe through a path containing spaces", () => {
    const directory = mkdtempSync(join(tmpdir(), "organon web native path "));
    try {
      const { resolved, packageName } = withPlatform("win32", "x64", () => ({
        resolved: resolveBinaryPath("web", {
          resolve: (specifier) => {
            expect(specifier).toBe("@tta-lab/pi-web-win32-x64/package.json");
            return join(directory, "node_modules", "@tta-lab", "pi-web-win32-x64", "package.json");
          },
        }),
        packageName: nativePackageName("web"),
      }));
      expect(resolved).toBe(
        join(directory, "node_modules", "@tta-lab", "pi-web-win32-x64", "bin", "web.exe"),
      );
      expect(packageName).toBe("@tta-lab/pi-web-win32-x64");
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  it("does not advertise Windows natives for other tools", () => {
    expect(() =>
      withPlatform("win32", "x64", () => resolveBinaryPath("project", { resolve: () => "" })),
    ).toThrow(/pi-web only/);
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

  it("terminates an already-started child when the signal aborts", async () => {
    const directory = mkdtempSync(join(tmpdir(), "pi-run-cli-"));
    const pidPath = join(directory, "pid");
    const controller = new AbortController();
    const pending = runCli(process.execPath, {
      args: [join(fixtures, "pid-hang.mjs"), pidPath],
      signal: controller.signal,
    });
    let pid = 0;

    try {
      await waitForFile(pidPath);
      pid = Number(readFileSync(pidPath, "utf8"));
      expect(pid).toBeGreaterThan(0);
      controller.abort();
      await expect(pending).rejects.toThrow("Operation aborted");
      await waitForProcessExit(pid);
    } finally {
      controller.abort();
      await pending.catch(() => undefined);
      if (pid > 0) {
        try {
          process.kill(pid, "SIGKILL");
        } catch {
          // The expected path is that SIGTERM from AbortSignal already exited it.
        }
      }
      rmSync(directory, { recursive: true, force: true });
    }
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

  it("normalizes cobra errors and empty stderr", async () => {
    expect((await cliError("Error: text not found\n", 1)).message).toBe("text not found");
    expect((await cliError("", 7)).message).toBe("command exited with code 7");
  });

  it("bounds oversized stderr and saves the original output", async () => {
    const stderr = "Error: " + "failure ".repeat(20_000) + "\nsecond diagnostic\n";
    const error = await cliError(stderr, 1);
    const fullOutputPath = error.message.match(/Full output saved to: ([^\]]+)/)?.[1];

    try {
      expect(Buffer.byteLength(error.message, "utf8")).toBeLessThanOrEqual(50 * 1024);
      expect(error.message).toContain("Inspect the saved stderr before retrying.");
      expect(fullOutputPath).toBeTruthy();
      expect(readFileSync(fullOutputPath!, "utf8")).toBe(stderr);
    } finally {
      if (fullOutputPath) {
        rmSync(dirname(fullOutputPath), { recursive: true, force: true });
      }
    }
  });
});
