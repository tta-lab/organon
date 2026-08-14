import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it, vi } from "vitest";

import { stageNativeBinary } from "../../shared/test/stage-native.js";

stageNativeBinary("src", fileURLToPath(new URL("./fixtures/bin/src", import.meta.url)));

// Intercept the queue helper so the mutation path's queue participation is
// observable at the adapter seam; the rest of the module keeps its real
// behavior.
vi.mock("@earendil-works/pi-coding-agent", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return {
    ...actual,
    withFileMutationQueue: vi.fn(async (_path: string, fn: () => Promise<unknown>) => fn()),
  };
});

import { srcTool } from "../src/tool.js";
import { withFileMutationQueue } from "@earendil-works/pi-coding-agent";

const mockedQueue = vi.mocked(withFileMutationQueue);

describe("pi-src mutation queue", () => {
  it("holds the resolved absolute target's file queue for the full child-process mutation window", async () => {
    const dir = mkdtempSync(join(tmpdir(), "pi-src-queue-"));
    const path = join(dir, "sample.go");
    writeFileSync(path, "package sample\n\nfunc Foo() {}\n");
    const def = srcTool();
    await def.execute("call-q", { action: "symbols", path } as any, undefined, undefined, {
      cwd: dir,
    } as any);
    mockedQueue.mockClear();
    await def.execute(
      "call-q",
      {
        action: "edit",
        path,
        edits: [{ oldText: "package sample", newText: "package example" }],
      } as any,
      undefined,
      undefined,
      { cwd: dir } as any,
    );
    expect(mockedQueue).toHaveBeenCalledTimes(1);
    expect(mockedQueue.mock.calls[0]![0]).toBe(path);
  });

  it("leaves read actions outside the mutation queue", async () => {
    const dir = mkdtempSync(join(tmpdir(), "pi-src-queue-"));
    const path = join(dir, "sample.go");
    writeFileSync(path, "package sample\n");
    const def = srcTool();
    mockedQueue.mockClear();
    await def.execute("call-q", { action: "read", path } as any, undefined, undefined, {
      cwd: dir,
    } as any);
    expect(mockedQueue).not.toHaveBeenCalled();
  });
});
