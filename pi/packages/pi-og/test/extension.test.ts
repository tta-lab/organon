import { readFileSync, rmSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import { ogSchema, ogTool } from "../src/tool.js";

const def = ogTool();
const ctx = { cwd: "/tmp" } as any;

function call(params: unknown) {
  return def.execute("call-1", params as any, undefined, undefined, ctx);
}

describe("pi-og extension", () => {
  it("registers one closed-union og tool with guidelines naming og", async () => {
    const { registerOgTool } = await import("../src/tool.js");
    const registered: any[] = [];
    registerOgTool({ registerTool: (d: any) => registered.push(d) } as any);
    expect(registered).toHaveLength(1);
    expect(registered[0]!.name).toBe("og");
  });

  it("validates the closed action union and action-specific constraints", () => {
    expect(Value.Check(ogSchema, { action: "auth_status", project: "ko" })).toBe(true);
    expect(Value.Check(ogSchema, { action: "push", project: "ko", force: true })).toBe(true);
    expect(Value.Check(ogSchema, { action: "clone", project: "ko" })).toBe(true);
    expect(Value.Check(ogSchema, { action: "clone", url: "https://github.com/a/b" })).toBe(true);
    expect(
      Value.Check(ogSchema, {
        action: "clone",
        url: "https://github.com/a/b",
        alias: "x",
        reference: true,
      }),
    ).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_create", project: "ko", title: "t" })).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_find", project: "ko", state: "closed" })).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_get", project: "ko", pr_id: 3 })).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_modify", project: "ko", title: "t" })).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_modify", project: "ko", body: "" })).toBe(true);
    expect(
      Value.Check(ogSchema, { action: "pr_modify", project: "ko", title: "t", body: "b" }),
    ).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_modify", project: "ko" })).toBe(false);
    expect(Value.Check(ogSchema, { action: "pr_comment", project: "ko", body: "note" })).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_log", project: "ko", tail: 100 })).toBe(true);
    expect(Value.Check(ogSchema, { action: "pr_get", project: "ko", pr_id: 0 })).toBe(false);
    expect(Value.Check(ogSchema, { action: "pr_log", project: "ko", tail: 2000 })).toBe(false);
    expect(Value.Check(ogSchema, { action: "pr_find", project: "ko", state: "bogus" })).toBe(false);
    expect(Value.Check(ogSchema, { action: "auth_status" })).toBe(false);
    expect(Value.Check(ogSchema, { action: "auth_status", project: "ko", bogus: 1 })).toBe(false);
  });

  it("auth_status passes the alias and returns the auth record", async () => {
    const result = await call({ action: "auth_status", project: "ko" });
    const details = result.details as {
      project: string;
      auth: { provider: string; ready: boolean };
    };
    expect(details.project).toBe("ko");
    expect(details.auth.provider).toBe("github");
    expect(details.auth.ready).toBe(true);
    expect((result.content[0] as { text: string }).text).toContain("Authenticated");
  });

  it("push/pull forward force and return the daemon message", async () => {
    const push = await call({ action: "push", project: "ko", force: true });
    expect((push.content[0] as { text: string }).text).toBe("push completed");
    const pull = await call({ action: "pull", project: "ko" });
    expect((pull.details as { project: string }).project).toBe("ko");
  });

  it("clone maps the project or url selector through to the CLI", async () => {
    const result = await call({ action: "clone", url: "https://github.com/a/b" });
    const details = result.details as { clone: { path: string; registered: boolean } };
    expect(details.clone.registered).toBe(true);
    expect((result.content[0] as { text: string }).text).toContain("Cloned");
  });

  it("pr_create sends the title and multiline body via stdin", async () => {
    const result = await call({
      action: "pr_create",
      project: "ko",
      title: "feat: x",
      body: "line1\nline2",
    });
    const details = result.details as { pr: { title: string; index: number } };
    expect(details.pr.title).toBe("feat: x");
    expect(details.pr.index).toBe(7);
  });

  it("pr_find validates state and passes it through", async () => {
    const result = await call({ action: "pr_find", project: "ko", state: "closed" });
    expect((result.details as { pr: { state: string } }).pr.state).toBe("open");
  });

  it("pr_get without pr_id uses the current-branch view; with pr_id uses the index", async () => {
    const current = await call({ action: "pr_get", project: "ko" });
    expect((current.details as { pr: { index: number } }).pr.index).toBe(7);
    const explicit = await call({ action: "pr_get", project: "ko", pr_id: 41 });
    expect((explicit.details as { pr: { index: number } }).pr.index).toBe(41);
  });

  it("pr_modify requires title or body and passes them", async () => {
    await expect(call({ action: "pr_modify", project: "ko" })).rejects.toThrow(/nothing to update/);
    const result = await call({ action: "pr_modify", project: "ko", title: "new title" });
    expect((result.details as { pr: { title: string } }).pr.title).toBe("new title");
  });

  it("pr_comment requires a non-blank body", async () => {
    await expect(call({ action: "pr_comment", project: "ko", body: "  " })).rejects.toThrow(
      /comment body must not be blank/,
    );
    const result = await call({ action: "pr_comment", project: "ko", pr_id: 41, body: "reviewed" });
    expect((result.details as { comment: { pr_id: number } }).comment.pr_id).toBe(41);
  });

  it("pr_log and pr_failures pass tail and return lines", async () => {
    const log = await call({ action: "pr_log", project: "ko", tail: 12 });
    const details = log.details as { lines: string[] };
    expect(details.lines.length).toBe(2);
    const failures = await call({ action: "pr_failures", project: "ko" });
    expect((failures.content[0] as { text: string }).text).toContain("check: pass");
  });

  it("preserves raw multiline PR bodies and comments through the CLI seam", async () => {
    const body = "\nbody with surrounding whitespace\n";
    const modified = await call({ action: "pr_modify", project: "ko", body });
    expect((modified.details as { pr: { body: string } }).pr.body).toBe(body);

    const comment = await call({ action: "pr_comment", project: "ko", body });
    expect((comment.details as { comment: { body: string } }).comment.body).toBe(body);
  });

  it("truncates and saves large messages, PR bodies, and CI output", async () => {
    const results = await Promise.all([
      call({ action: "push", project: "large" }),
      call({ action: "pr_get", project: "large" }),
      call({ action: "pr_log", project: "large" }),
    ]);
    const paths = results.map(
      (result) => (result.details as { fullOutputPath?: string }).fullOutputPath,
    );
    try {
      for (const result of results) {
        const text = (result.content[0] as { text: string }).text;
        const details = result.details as { fullOutputPath?: string };
        expect(text).toContain("[Truncated:");
        expect(text).toContain(`Full output saved to: ${details.fullOutputPath}`);
        expect(readFileSync(details.fullOutputPath!, "utf8")).toContain("line 2999");
      }
    } finally {
      for (const path of paths) {
        if (path) {
          rmSync(dirname(path), { recursive: true, force: true });
        }
      }
    }
  });

  it("daemon policy failures surface as concise errors", async () => {
    await expect(call({ action: "push", project: "ko", force: false })).resolves.toBeTruthy();
  });

  it("abort cancels the CLI child process", async () => {
    const controller = new AbortController();
    controller.abort();
    await expect(
      def.execute(
        "call-2",
        { action: "auth_status", project: "ko" },
        controller.signal,
        undefined,
        ctx,
      ),
    ).rejects.toThrow("Operation aborted");
  });
});
