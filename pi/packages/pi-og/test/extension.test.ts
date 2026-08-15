import { readFileSync, rmSync } from "node:fs";
import { dirname } from "node:path";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import {
  ogAuthStatusSchema,
  ogAuthStatusTool,
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
} from "../src/tool.js";

const definitions = {
  auth_status: ogAuthStatusTool(),
  clone: ogCloneTool(),
  pull: ogPullTool(),
  push: ogPushTool(),
  pr: ogPrTool(),
  checks: ogChecksTool(),
} as const;

type ToolName = keyof typeof definitions;
const ctx = { cwd: "/tmp" } as any;
const emptyProjectInputs: Array<[ToolName, unknown]> = [
  ["auth_status", { project: "" }],
  ["pull", { project: "" }],
  ["push", { project: "" }],
  ["clone", { project: "" }],
  ["pr", { action: "create", project: "", title: "title" }],
  ["pr", { action: "find", project: "" }],
  ["pr", { action: "get", project: "" }],
  ["pr", { action: "modify", project: "", title: "title" }],
  ["pr", { action: "comment", project: "", body: "body" }],
  ["checks", { action: "status", project: "" }],
  ["checks", { action: "log", project: "" }],
  ["checks", { action: "failures", project: "" }],
];

function call(tool: ToolName, params: unknown) {
  return definitions[tool].execute("call-1", params, undefined, undefined, ctx);
}

describe("pi-og extension", () => {
  it("registers exactly the six capability-oriented tools and no mega-tool", async () => {
    const { registerOgTools } = await import("../src/tool.js");
    const registered: any[] = [];
    registerOgTools({ registerTool: (d: any) => registered.push(d) } as any);
    expect(registered.map((definition) => definition.name)).toEqual([
      "og_auth_status",
      "og_clone",
      "og_pull",
      "og_push",
      "og_pr",
      "og_checks",
    ]);
    expect(registered.every((definition) => definition.name !== "og")).toBe(true);
  });

  it("uses direct schemas for standalone tools and closed action unions for grouped tools", () => {
    expect(Value.Check(ogAuthStatusSchema, { project: "ko" })).toBe(true);
    expect(Value.Check(ogAuthStatusSchema, {})).toBe(false);
    expect(Value.Check(ogAuthStatusSchema, { project: "ko", action: "status" })).toBe(false);
    expect(Value.Check(ogPullSchema, { project: "ko" })).toBe(true);
    expect(Value.Check(ogPullSchema, {})).toBe(false);
    expect(Value.Check(ogPullSchema, { project: "ko", bogus: 1 })).toBe(false);
    expect(Value.Check(ogPushSchema, { project: "ko", force: true })).toBe(true);
    expect(Value.Check(ogPushSchema, {})).toBe(false);
    expect(Value.Check(ogPushSchema, { project: "ko", force: true, bogus: 1 })).toBe(false);
    expect(Value.Check(ogCloneSchema, { project: "ko" })).toBe(true);
    expect(Value.Check(ogCloneSchema, {})).toBe(false);
    expect(Value.Check(ogCloneSchema, { url: "" })).toBe(false);
    expect(Value.Check(ogCloneSchema, { url: "https://github.com/a/b" })).toBe(true);
    expect(
      Value.Check(ogCloneSchema, {
        url: "https://github.com/a/b",
        alias: "x",
        reference: true,
      }),
    ).toBe(true);
    expect(Value.Check(ogCloneSchema, { project: "ko", url: "https://github.com/a/b" })).toBe(
      false,
    );
    expect(Value.Check(ogCloneSchema, { url: "https://github.com/a/b", bogus: 1 })).toBe(false);

    expect(Value.Check(ogPrSchema, { action: "create", project: "ko", title: "t" })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "find", project: "ko", state: "closed" })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "get", project: "ko", pr_id: 3 })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "get", project: "ko", pr_id: 0 })).toBe(false);
    expect(Value.Check(ogPrSchema, { action: "modify", project: "ko", title: "t" })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "modify", project: "ko", body: "" })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "modify", project: "ko" })).toBe(false);
    expect(Value.Check(ogPrSchema, { action: "comment", project: "ko", body: "note" })).toBe(true);
    expect(Value.Check(ogPrSchema, { action: "status", project: "ko" })).toBe(false);
    expect(Value.Check(ogPrSchema, { action: "get", project: "ko", bogus: 1 })).toBe(false);

    expect(Value.Check(ogChecksSchema, { action: "status", project: "ko" })).toBe(true);
    expect(Value.Check(ogChecksSchema, { action: "status" })).toBe(false);
    expect(Value.Check(ogChecksSchema, { action: "status", project: "ko", pr_id: 0 })).toBe(false);
    expect(Value.Check(ogChecksSchema, { action: "status", project: "ko", bogus: 1 })).toBe(false);
    expect(Value.Check(ogChecksSchema, { action: "log", project: "ko", tail: 100 })).toBe(true);
    expect(Value.Check(ogChecksSchema, { action: "failures", project: "ko" })).toBe(true);
    expect(Value.Check(ogChecksSchema, { action: "get", project: "ko" })).toBe(false);
    expect(Value.Check(ogChecksSchema, { action: "status", project: "ko", tail: 1 })).toBe(false);
    expect(Value.Check(ogChecksSchema, { action: "log", project: "ko", pr_id: 0 })).toBe(false);
    expect(Value.Check(ogChecksSchema, { action: "log", project: "ko", tail: 2000 })).toBe(false);
    expect(Value.Check(ogPrSchema, { action: "find", project: "ko", state: "bogus" })).toBe(false);
    expect(Value.Check(ogAuthStatusSchema, { project: "ko", bogus: 1 })).toBe(false);

    for (const [tool, input] of emptyProjectInputs) {
      const schema = definitions[tool].parameters;
      expect(Value.Check(schema, input)).toBe(false);
    }
  });

  it("rejects empty project references for every registered-project tool before starting the CLI", async () => {
    for (const [tool, input] of emptyProjectInputs) {
      await expect(call(tool, input)).rejects.toThrow(/project reference must not be empty/);
    }
  });

  it("passes project references through and renders the canonical alias returned by OG", async () => {
    const auth = await call("auth_status", { project: "flick-backend" });
    expect((auth.details as { project: string }).project).toBe("fb");
    const push = await call("push", { project: "flick-backend" });
    expect((push.details as { project: string }).project).toBe("fb");
  });

  it("auth_status works with only the package-local fixture binary", async () => {
    const result = await call("auth_status", { project: "ko" });
    const details = result.details as {
      project: string;
      auth: { provider: string; ready: boolean };
    };
    expect(details.project).toBe("ko");
    expect(details.auth.provider).toBe("github");
    expect(details.auth.ready).toBe(true);
    expect((result.content[0] as { text: string }).text).toContain("Authenticated");
  });

  it("push/pull forward force and return the OG message", async () => {
    const push = await call("push", { project: "ko", force: true });
    expect((push.content[0] as { text: string }).text).toBe("ko: push completed");
    expect((push.details as { force_with_lease: boolean }).force_with_lease).toBe(true);
    const pull = await call("pull", { project: "ko" });
    expect((pull.details as { project: string }).project).toBe("ko");
  });

  it("clone maps project, alias, and reference selectors through to the CLI", async () => {
    const project = await call("clone", { project: "ko" });
    expect((project.details as { clone: { registered: boolean } }).clone.registered).toBe(true);

    const aliased = await call("clone", {
      url: "https://github.com/a/b",
      alias: "example",
    });
    expect((aliased.details as { clone: { alias: string } }).clone.alias).toBe("example");

    const reference = await call("clone", {
      url: "https://github.com/a/b",
      reference: true,
    });
    const details = reference.details as { clone: { path: string; registered: boolean } };
    expect(details.clone.registered).toBe(false);
    expect((reference.content[0] as { text: string }).text).toContain("Cloned");
  });

  it("pr_create sends the title and multiline body via stdin", async () => {
    const result = await call("pr", {
      action: "create",
      project: "ko",
      title: "feat: x",
      body: "line1\nline2",
    });
    const details = result.details as { pr: { title: string; index: number } };
    expect(details.pr.title).toBe("feat: x");
    expect(details.pr.index).toBe(7);
  });

  it("pr_find validates state and passes it through", async () => {
    const result = await call("pr", { action: "find", project: "ko", state: "closed" });
    expect((result.details as { pr: { state: string } }).pr.state).toBe("open");
  });

  it("pr_get without pr_id uses the current-branch view; with pr_id uses the index", async () => {
    const current = await call("pr", { action: "get", project: "ko" });
    expect((current.details as { pr: { index: number } }).pr.index).toBe(7);
    const explicit = await call("pr", { action: "get", project: "ko", pr_id: 41 });
    expect((explicit.details as { pr: { index: number } }).pr.index).toBe(41);
  });

  it("pr_modify requires title or body and passes them", async () => {
    await expect(call("pr", { action: "modify", project: "ko" })).rejects.toThrow(
      /nothing to update/,
    );
    const result = await call("pr", { action: "modify", project: "ko", title: "new title" });
    expect((result.details as { pr: { title: string } }).pr.title).toBe("new title");
  });

  it("pr_comment requires a non-blank body", async () => {
    await expect(call("pr", { action: "comment", project: "ko", body: "  " })).rejects.toThrow(
      /comment body must not be blank/,
    );
    const result = await call("pr", {
      action: "comment",
      project: "ko",
      pr_id: 41,
      body: "reviewed",
    });
    expect((result.details as { comment: { pr_id: number } }).comment.pr_id).toBe(41);
  });

  it("og_checks maps status, log, and failures to the existing CLI operations", async () => {
    const status = await call("checks", { action: "status", project: "ko" });
    expect((status.details as { pr: { index: number } }).pr.index).toBe(7);
    expect((status.content[0] as { text: string }).text).toContain("check: pass");
    const log = await call("checks", { action: "log", project: "ko", tail: 12 });
    const details = log.details as { lines: string[] };
    expect(details.lines.length).toBe(2);
    const failures = await call("checks", { action: "failures", project: "ko" });
    expect((failures.content[0] as { text: string }).text).toContain("check: pass");
  });

  it("preserves raw multiline PR bodies and comments through the CLI seam", async () => {
    const body = "\nbody with surrounding whitespace\n";
    const modified = await call("pr", { action: "modify", project: "ko", body });
    expect((modified.details as { pr: { body: string } }).pr.body).toBe(body);

    const comment = await call("pr", { action: "comment", project: "ko", body });
    expect((comment.details as { comment: { body: string } }).comment.body).toBe(body);
  });

  it("truncates and saves large messages, PR bodies, and CI output", async () => {
    const results = await Promise.all([
      call("push", { project: "large" }),
      call("pr", { action: "get", project: "large" }),
      call("checks", { action: "log", project: "large" }),
    ]);
    const paths = results.map(
      (result) => (result.details as { fullOutputPath?: string }).fullOutputPath,
    );
    try {
      const hints = ["Use og_push", "Use og_pr with action get", "Use og_checks with action log"];
      results.forEach((result, index) => {
        const text = (result.content[0] as { text: string }).text;
        const details = result.details as { fullOutputPath?: string };
        expect(text).toContain("[Truncated:");
        expect(text).toContain(hints[index]!);
        expect(text).toContain(`Full output saved to: ${details.fullOutputPath}`);
        expect(readFileSync(details.fullOutputPath!, "utf8")).toContain("line 2999");
      });
    } finally {
      for (const path of paths) {
        if (path) {
          rmSync(dirname(path), { recursive: true, force: true });
        }
      }
    }
  });

  it("CLI policy failures retain the existing concise error behavior", async () => {
    await expect(call("push", { project: "failure", force: false })).rejects.toThrow(
      /OG policy rejected the operation/,
    );
  });

  it("abort cancels the CLI child process", async () => {
    const controller = new AbortController();
    controller.abort();
    await expect(
      definitions.auth_status.execute(
        "call-2",
        { project: "ko" },
        controller.signal,
        undefined,
        ctx,
      ),
    ).rejects.toThrow("Operation aborted");
  });
});
