import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import {
  ogChecksSchema,
  ogChecksTool,
  ogCloneSchema,
  ogCloneTool,
  ogPrSchema,
  ogPrTool,
  ogPullSchema,
  ogPullTool,
  ogPushSchema,
  ogPushTool,
  registerOgTools,
} from "../src/tool.js";

const definitions = {
  clone: ogCloneTool(),
  pull: ogPullTool(),
  push: ogPushTool(),
  pr: ogPrTool(),
  checks: ogChecksTool(),
} as const;
type ToolName = keyof typeof definitions;
const ctx = { cwd: "/tmp" } as any;

function call(tool: ToolName, params: unknown) {
  return definitions[tool].execute("call-1", params, undefined, undefined, ctx);
}

async function expectOgRejectedBeforeBinary(
  tool: ToolName,
  params: unknown,
  error: RegExp,
): Promise<void> {
  const directory = mkdtempSync(join(tmpdir(), "pi-og-invalid-"));
  const invocationPath = join(directory, "invocations");
  const priorInvocationPath = process.env.PI_OG_TEST_INVOCATIONS;
  process.env.PI_OG_TEST_INVOCATIONS = invocationPath;
  try {
    await expect(call(tool, params)).rejects.toThrow(error);
    expect(existsSync(invocationPath)).toBe(false);
  } finally {
    if (priorInvocationPath === undefined) delete process.env.PI_OG_TEST_INVOCATIONS;
    else process.env.PI_OG_TEST_INVOCATIONS = priorInvocationPath;
    rmSync(directory, { recursive: true, force: true });
  }
}

describe("pi-og extension", () => {
  it("registers exactly the five model-facing capabilities and no auth-status tool", () => {
    const registered: any[] = [];
    registerOgTools({ registerTool: (definition: any) => registered.push(definition) });
    expect(registered.map((definition) => definition.name)).toEqual([
      "og_clone",
      "og_pull",
      "og_push",
      "og_pr",
      "og_checks",
    ]);
  });

  it("uses direct schemas and documents grouped action fields without a root union", () => {
    for (const schema of [ogCloneSchema, ogPullSchema, ogPushSchema, ogPrSchema, ogChecksSchema]) {
      expect((schema as any).type).toBe("object");
      expect((schema as any).anyOf).toBeUndefined();
      expect((schema as any).oneOf).toBeUndefined();
    }
    expect(Value.Check(ogPullSchema, { project: "ko" })).toBe(true);
    expect(Value.Check(ogPushSchema, { project: "ko", force: true })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "create", project: "ko", title: "t" })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "modify", project: "ko" })).toBe(true);
    expect(Value.Check(ogChecksSchema, { action: "status", project: "ko", tail: 1 })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "nope", project: "ko" })).toBe(false);
    expect(Value.Check(ogChecksSchema, { action: "log", project: "ko", tail: 2000 })).toBe(false);
  });

  it("preserves clone target modes and exact-one validation", async () => {
    expect(Value.Check(ogCloneSchema, { project: "ko" })).toBe(true);
    expect(
      Value.Check(ogCloneSchema, {
        url: "https://github.com/a/b",
        alias: "example",
        reference: true,
      }),
    ).toBe(true);
    await expect(call("clone", {})).rejects.toThrow(/exactly one/);
    await expect(call("clone", { project: "ko", url: "https://github.com/a/b" })).rejects.toThrow(
      /exactly one/,
    );
    await expect(call("clone", { project: "ko", alias: "bad" })).rejects.toThrow(
      /does not accept field alias/,
    );

    const project = await call("clone", { project: "ko" });
    expect((project.details as { clone: { registered: boolean } }).clone.registered).toBe(true);
    const aliased = await call("clone", { url: "https://github.com/a/b", alias: "example" });
    expect((aliased.details as { clone: { alias: string } }).clone.alias).toBe("example");
  });

  it("keeps pull, push, multiline PR operations, and checks behavior", async () => {
    const push = await call("push", { project: "ko", force: true });
    expect((push.content[0] as { text: string }).text).toBe("ko: push completed");
    expect((push.details as { force_with_lease: boolean }).force_with_lease).toBe(true);
    const pull = await call("pull", { project: "ko" });
    expect((pull.details as { project: string }).project).toBe("ko");

    const created = await call("pr", {
      action: "create",
      project: "ko",
      title: "feat: x",
      body: "line1\nline2",
    });
    expect((created.details as { pr: { title: string } }).pr.title).toBe("feat: x");
    const found = await call("pr", { action: "find", project: "ko", state: "closed" });
    expect((found.details as { pr: { state: string } }).pr.state).toBe("closed");
    const current = await call("pr", { action: "get", project: "ko" });
    expect((current.details as { pr: { index: number } }).pr.index).toBe(7);
    const modified = await call("pr", { action: "modify", project: "ko", body: "\nnew body\n" });
    expect((modified.details as { pr: { body: string } }).pr.body).toBe("\nnew body\n");
    const comment = await call("pr", {
      action: "comment",
      project: "ko",
      pr_id: 41,
      body: "reviewed",
    });
    expect((comment.details as { comment: { pr_id: number } }).comment.pr_id).toBe(41);

    const status = await call("checks", { action: "status", project: "ko", pr_id: 41 });
    expect((status.details as { pr: { index: number } }).pr.index).toBe(41);
    const log = await call("checks", { action: "log", project: "ko", tail: 1 });
    expect((log.details as { lines: string[] }).lines).toHaveLength(1);
    const failures = await call("checks", { action: "failures", project: "ko" });
    expect((failures.details as { lines: string[] }).lines).toHaveLength(2);
  });

  it("rejects grouped-action missing, irrelevant, malformed, and cross-action fields before the CLI", async () => {
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "create", project: "ko" },
      /title must not be blank/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "comment", project: "ko" },
      /comment body must not be blank/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "modify", project: "ko" },
      /nothing to update/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "find", project: "ko", title: "wrong" },
      /does not accept field title/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "get", project: "ko", state: "closed" },
      /does not accept field state/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "comment", project: "ko", title: "wrong", body: "comment" },
      /does not accept field title/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "create", project: "ko", title: 42 },
      /title must not be blank/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "find", project: "ko", state: "invalid" },
      /state must be open, closed, or all/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "get", project: "ko", pr_id: 0 },
      /pr_id must be a positive integer/,
    );
    await expectOgRejectedBeforeBinary(
      "pr",
      { action: "comment", project: "ko", body: 42 },
      /comment body must not be blank/,
    );

    await expectOgRejectedBeforeBinary("checks", { project: "ko" }, /action is required/);
    await expectOgRejectedBeforeBinary(
      "checks",
      { action: "status" },
      /project reference must not be empty/,
    );
    await expectOgRejectedBeforeBinary(
      "checks",
      { action: "status", project: "ko", tail: 1 },
      /does not accept field tail/,
    );
    await expectOgRejectedBeforeBinary(
      "checks",
      { action: "log", project: "ko", tail: 2001 },
      /tail must be an integer between 0 and 1000/,
    );
    await expectOgRejectedBeforeBinary(
      "checks",
      { action: "failures", project: "ko", pr_id: 0 },
      /pr_id must be a positive integer/,
    );
    await expectOgRejectedBeforeBinary(
      "checks",
      { action: "unknown", project: "ko" },
      /action must be status, log, or failures/,
    );

    await expectOgRejectedBeforeBinary(
      "push",
      { project: "" },
      /project reference must not be empty/,
    );
  });

  it("retains model truncation, policy errors, and cancellation", async () => {
    const results = await Promise.all([
      call("push", { project: "large" }),
      call("pr", { action: "get", project: "large" }),
      call("checks", { action: "log", project: "large" }),
    ]);
    const paths = results.map(
      (result) => (result.details as { fullOutputPath?: string }).fullOutputPath,
    );
    try {
      results.forEach((result) => {
        const details = result.details as { fullOutputPath?: string };
        expect((result.content[0] as { text: string }).text).toContain("[Truncated:");
        expect(readFileSync(details.fullOutputPath!, "utf8")).toContain("line 2999");
      });
    } finally {
      for (const path of paths) if (path) rmSync(dirname(path), { recursive: true, force: true });
    }
    await expect(call("push", { project: "failure" })).rejects.toThrow(/OG policy rejected/);
    const controller = new AbortController();
    controller.abort();
    await expect(
      definitions.pull.execute("call-2", { project: "ko" }, controller.signal, undefined, ctx),
    ).rejects.toThrow("Operation aborted");
  });
});
