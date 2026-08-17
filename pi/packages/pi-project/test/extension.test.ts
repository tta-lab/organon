import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import {
  projectFindSchema,
  projectFindTool,
  projectGetSchema,
  projectGetTool,
  projectListSchema,
  projectListTool,
  registerProjectTools,
} from "../src/tool.js";

const definitions = {
  list: projectListTool(),
  find: projectFindTool(),
  get: projectGetTool(),
} as const;
type ToolName = keyof typeof definitions;
const ctx = { cwd: "/tmp", model: undefined } as any;

function call(tool: ToolName, params: unknown) {
  return definitions[tool].execute("call-1", params as any, undefined, undefined, ctx);
}

describe("pi-project extension", () => {
  it("registers project list, find, and get without the old mega-tool", () => {
    const registered: any[] = [];
    registerProjectTools({ registerTool: (definition: any) => registered.push(definition) });
    expect(registered.map((definition) => definition.name)).toEqual([
      "project_list",
      "project_find",
      "project_get",
    ]);
  });

  it("uses closed direct schemas with the expected required fields", () => {
    expect((projectListSchema as any).type).toBe("object");
    expect((projectFindSchema as any).type).toBe("object");
    expect((projectGetSchema as any).type).toBe("object");
    expect(Value.Check(projectListSchema, {})).toBe(true);
    expect(Value.Check(projectListSchema, { bogus: 1 })).toBe(false);
    expect(Value.Check(projectFindSchema, { query: "runtime" })).toBe(true);
    expect(Value.Check(projectFindSchema, { query: "", limit: 0 })).toBe(false);
    expect(Value.Check(projectGetSchema, { project: "len" })).toBe(true);
    expect(Value.Check(projectGetSchema, {})).toBe(false);
    expect(Value.Check(projectGetSchema, { alias: "len" })).toBe(false);
  });

  it("lists active and archived projects with structured details", async () => {
    const active = await call("list", {});
    expect((active.content[0] as { text: string }).text).toContain(
      "- len: Lenos CLI runtime (/home/neil/code/projects/tta-lab/lenos)",
    );
    expect((active.content[0] as { text: string }).text).not.toContain("[archived]");
    expect(
      (active.details as { projects: Array<{ archived: boolean }> }).projects.every(
        (p) => !p.archived,
      ),
    ).toBe(true);

    const all = await call("list", { include_archived: true });
    expect(
      (all.details as { projects: Array<{ alias: string }> }).projects.map((p) => p.alias),
    ).toEqual(["len", "orientation", "ttal"]);
  });

  it("finds active projects and gets exact references", async () => {
    const found = await call("find", { query: "runtime", limit: 16 });
    expect(
      (found.details as { projects: Array<{ alias: string }> }).projects.map((p) => p.alias),
    ).toEqual(["len"]);
    expect((found.content[0] as { text: string }).text).toContain(
      "- len: Lenos CLI runtime (/home/neil/code/projects/tta-lab/lenos)",
    );

    const empty = await call("find", { query: "missing" });
    expect((empty.details as { projects: unknown[] }).projects).toEqual([]);
    expect((empty.content[0] as { text: string }).text).toContain("No active projects found");

    const result = await call("get", { project: "lenos" });
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

  it("rejects blank references before starting the CLI", async () => {
    await expect(call("get", { project: "" })).rejects.toThrow(
      /project must be a non-blank string/,
    );
    await expect(call("find", { query: "" })).rejects.toThrow(/query must be a non-blank string/);
  });
});
