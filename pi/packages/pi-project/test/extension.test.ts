import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import { projectSchema, projectTool } from "../src/tool.js";

type ToolDef = ReturnType<typeof projectTool>;

function fakePi() {
  const registered: ToolDef[] = [];
  return {
    registered,
    registerTool(def: ToolDef) {
      registered.push(def);
    },
  };
}

function ctx() {
  return { cwd: "/tmp", model: undefined } as any;
}

async function callExecute(def: ToolDef, params: unknown) {
  return def.execute("call-1", params as any, undefined, undefined, ctx());
}

describe("pi-project extension", () => {
  it("registers exactly one global tool named project with prompt metadata", async () => {
    const pi = fakePi();
    const { registerProjectTool } = await import("../src/tool.js");
    registerProjectTool(pi as any);
    expect(pi.registered).toHaveLength(1);
    const def = pi.registered[0]!;
    expect(def.name).toBe("project");
  });

  it("exposes a closed action union: unknown fields rejected, get requires project reference", () => {
    expect(Value.Check(projectSchema, { action: "list" })).toBe(true);
    expect(Value.Check(projectSchema, { action: "list", include_archived: true })).toBe(true);
    expect(Value.Check(projectSchema, { action: "get", project: "len" })).toBe(true);
    expect(Value.Check(projectSchema, { action: "get" })).toBe(false);
    expect(Value.Check(projectSchema, { action: "get", alias: "len" })).toBe(false);
    expect(Value.Check(projectSchema, { action: "list", bogus: 1 })).toBe(false);
    expect(Value.Check(projectSchema, { action: "nope" })).toBe(false);
    expect(Value.Check(projectSchema, {})).toBe(false);
  });

  it("list action calls the package binary and converts JSON to text plus details", async () => {
    const def = projectTool();
    const result = await callExecute(def, { action: "list" });
    expect(result.content[0]!.type).toBe("text");
    const text = (result.content[0] as { text: string }).text;
    expect(text).toContain("len");
    expect(text).not.toContain("[archived]");
    const details = result.details as { projects: Array<{ alias: string; archived: boolean }> };
    expect(details.projects.length).toBe(2);
    expect(details.projects.every((p) => !p.archived)).toBe(true);
  });

  it("list with include_archived passes the flag through", async () => {
    const def = projectTool();
    const result = await callExecute(def, { action: "list", include_archived: true });
    const details = result.details as { projects: Array<{ alias: string }> };
    expect(details.projects.map((p) => p.alias)).toEqual(["len", "orientation", "ttal"]);
  });

  it("get action returns the five-field project record in details", async () => {
    const def = projectTool();
    const result = await callExecute(def, { action: "get", project: "len" });
    const details = result.details as { project: Record<string, unknown> };
    expect(details.project.alias).toBe("len");
    expect(Object.keys(details.project).sort()).toEqual([
      "alias",
      "archived",
      "name",
      "path",
      "remote",
    ]);
  });

  it("uses project references and exposes bounded active find", () => {
    expect(Value.Check(projectSchema, { action: "get", project: "lenos" })).toBe(true);
    expect(Value.Check(projectSchema, { action: "get", alias: "len" })).toBe(false);
    expect(Value.Check(projectSchema, { action: "find", query: "runtime" })).toBe(true);
    expect(Value.Check(projectSchema, { action: "find", query: "runtime", limit: 16 })).toBe(true);
    expect(Value.Check(projectSchema, { action: "find", query: "runtime", limit: 0 })).toBe(false);
    expect(Value.Check(projectSchema, { action: "find", query: "runtime", limit: 33 })).toBe(true);
    expect(Value.Check(projectSchema, { action: "find", query: "" })).toBe(false);
    expect(Value.Check(projectSchema, { action: "find" })).toBe(false);
    expect(
      Value.Check(projectSchema, { action: "find", query: "runtime", include_archived: true }),
    ).toBe(false);
  });

  it("gets by a project reference and finds active projects with empty success", async () => {
    const get = await callExecute(projectTool(), { action: "get", project: "lenos" });
    expect((get.details as { project: { alias: string } }).project.alias).toBe("len");
    expect((get.content[0] as { text: string }).text).toContain("len:");

    const found = await callExecute(projectTool(), { action: "find", query: "runtime" });
    expect(
      (found.details as { projects: Array<{ alias: string }> }).projects.map((p) => p.alias),
    ).toEqual(["len"]);
    expect((found.content[0] as { text: string }).text).toContain("len");

    const limited = await callExecute(projectTool(), {
      action: "find",
      query: "limited",
      limit: 3,
    });
    expect((limited.details as { projects: Array<{ alias: string }> }).projects[0]?.alias).toBe(
      "len",
    );

    const empty = await callExecute(projectTool(), { action: "find", query: "missing" });
    expect((empty.details as { projects: unknown[] }).projects).toEqual([]);
    expect((empty.content[0] as { text: string }).text).toContain("No active projects found");
  });

  it("turns a nonzero exit into a concise error from stderr", async () => {
    const def = projectTool();
    await expect(callExecute(def, { action: "get", project: "missing" })).rejects.toThrow(
      /project "missing" not found/,
    );
  });

  it("propagates abort to the child and rejects with Operation aborted", async () => {
    const def = projectTool();
    const controller = new AbortController();
    controller.abort();
    await expect(
      def.execute("call-2", { action: "list" }, controller.signal, undefined, ctx()),
    ).rejects.toThrow("Operation aborted");
  });
});
