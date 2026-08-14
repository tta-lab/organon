import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import { stageNativeBinary } from "../../shared/test/stage-native.js";
import { projectSchema, projectTool } from "../src/tool.js";

stageNativeBinary("project", fileURLToPath(new URL("./fixtures/bin/project", import.meta.url)));

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
    expect(def.label).toBe("Project");
    expect(def.description).toContain("registered");
    expect(def.promptSnippet).toBeTruthy();
    expect(def.promptGuidelines).toBeTruthy();
    expect(def.promptGuidelines!.every((g) => g.includes("project"))).toBe(true);
  });

  it("exposes a closed action union: unknown fields rejected, get requires alias", () => {
    expect(Value.Check(projectSchema, { action: "list" })).toBe(true);
    expect(Value.Check(projectSchema, { action: "list", include_archived: true })).toBe(true);
    expect(Value.Check(projectSchema, { action: "get", alias: "len" })).toBe(true);
    expect(Value.Check(projectSchema, { action: "get" })).toBe(false);
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
    const result = await callExecute(def, { action: "get", alias: "len" });
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

  it("turns a nonzero exit into a concise error from stderr", async () => {
    const def = projectTool();
    await expect(callExecute(def, { action: "get", alias: "missing" })).rejects.toThrow(
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
