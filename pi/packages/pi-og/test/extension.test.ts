import { readFileSync, rmSync } from "node:fs";
import { dirname } from "node:path";

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
    expect(registered.every((definition) => definition.name !== "og_auth_status")).toBe(true);
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
  });

  it("rejects invalid grouped combinations before execution", async () => {
    await expect(call("pr", { action: "create", project: "ko" })).rejects.toThrow(
      /title must not be blank/,
    );
    await expect(call("pr", { action: "find", project: "ko", title: "wrong" })).rejects.toThrow(
      /does not accept field title/,
    );
    await expect(call("pr", { action: "modify", project: "ko" })).rejects.toThrow(
      /nothing to update/,
    );
    await expect(call("pr", { action: "comment", project: "ko", body: "  " })).rejects.toThrow(
      /comment body must not be blank/,
    );
    await expect(call("checks", { action: "status", project: "ko", tail: 1 })).rejects.toThrow(
      /does not accept field tail/,
    );
    await expect(call("checks", { action: "log", project: "ko", tail: 2001 })).rejects.toThrow(
      /tail must be/,
    );
    await expect(call("push", { project: "" })).rejects.toThrow(
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
