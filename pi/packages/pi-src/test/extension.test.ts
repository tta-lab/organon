import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import { renderReadText, srcSchema, srcTool } from "../src/tool.js";
import { createTakeoverHandlers, applyReadTakeover, restoreReadTakeover } from "../src/takeover.js";
import { resolveSourcePath } from "../src/paths.js";

const def = srcTool();

function makeFile(content: string): { path: string; cwd: string } {
  const dir = mkdtempSync(join(tmpdir(), "pi-src-"));
  const path = join(dir, "sample.go");
  writeFileSync(path, content);
  return { path, cwd: dir };
}

const SAMPLE =
  "package sample\n\n// Foo docs.\nfunc Foo() {\n\treturn 1\n}\n\nfunc Bar() {\n\treturn 2\n}\n";

async function call(params: unknown, ctx: { cwd: string }) {
  return def.execute("call-1", params as any, undefined, undefined, ctx);
}

describe("pi-src schema", () => {
  it("exposes a closed action union with action-specific required fields", () => {
    expect(Value.Check(srcSchema, { action: "symbols", path: "a.go" })).toBe(true);
    expect(
      Value.Check(srcSchema, {
        action: "read",
        path: "a.go",
        symbol_id: "bK",
        offset: 1,
        limit: 10,
      }),
    ).toBe(true);
    expect(Value.Check(srcSchema, { action: "read", path: "a.go", offset: 0 })).toBe(false);
    expect(Value.Check(srcSchema, { action: "read", path: "a.go", offset: 1.5 })).toBe(false);
    expect(Value.Check(srcSchema, { action: "read", path: "a.go", limit: -1 })).toBe(false);
    expect(
      Value.Check(srcSchema, { action: "replace", path: "a.go", symbol_id: "bK", content: "x" }),
    ).toBe(true);
    expect(
      Value.Check(srcSchema, {
        action: "insert",
        path: "a.go",
        symbol_id: "bK",
        position: "before",
        content: "x",
      }),
    ).toBe(true);
    expect(Value.Check(srcSchema, { action: "delete", path: "a.go", symbol_id: "bK" })).toBe(true);
    expect(
      Value.Check(srcSchema, { action: "comment", path: "a.go", symbol_id: "bK", read: true }),
    ).toBe(true);
    expect(
      Value.Check(srcSchema, {
        action: "comment",
        path: "a.go",
        symbol_id: "bK",
        content: "// new",
      }),
    ).toBe(true);
    expect(
      Value.Check(srcSchema, {
        action: "edit",
        path: "a.go",
        edits: [{ oldText: "a", newText: "b" }],
      }),
    ).toBe(true);
    expect(Value.Check(srcSchema, { action: "edit", path: "a.go", edits: [] })).toBe(false);
    expect(Value.Check(srcSchema, { action: "symbols" })).toBe(false);
    expect(Value.Check(srcSchema, { action: "replace", path: "a.go" })).toBe(false);
    expect(Value.Check(srcSchema, { action: "read", path: "a.go", bogus: 1 })).toBe(false);
    expect(Value.Check(srcSchema, { action: "nope", path: "a.go" })).toBe(false);
  });
});

describe("pi-src read and symbols", () => {
  it("resolves relative paths against ctx.cwd and strips a leading @", () => {
    const { path, cwd } = makeFile(SAMPLE);
    const relative = join(cwd, "sample.go");
    expect(resolveSourcePath("@" + relative, cwd)).toBe(path);
    expect(resolveSourcePath(relative, cwd)).toBe(path);
    expect(resolveSourcePath(path, cwd)).toBe(path);
  });

  it("symbols action returns the typed outline with opaque IDs", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const result = await call({ action: "symbols", path }, { cwd });
    const details = result.details as {
      language: string;
      symbols: Array<{ id: string; name: string; has_doc: boolean }>;
    };
    expect(details.language).toBe("go");
    expect(details.symbols.map((s) => s.name)).toEqual(["Foo", "Bar"]);
    expect(details.symbols[0]!.has_doc).toBe(true);
    expect(details.symbols[0]!.id).not.toBe("Foo");
  });

  it("read whole file returns content with truncation details", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const result = await call({ action: "read", path }, { cwd });
    expect((result.content[0] as { text: string }).text).toContain("func Foo");
    expect((result.details as { truncation?: unknown }).truncation).toBeUndefined();
  });

  it("read by exact symbol ID returns that symbol only", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const symbols = await call({ action: "symbols", path }, { cwd });
    const foo = (symbols.details as { symbols: Array<{ id: string; name: string }> }).symbols.find(
      (s) => s.name === "Foo",
    )!;
    const result = await call({ action: "read", path, symbol_id: foo.id }, { cwd });
    const text = (result.content[0] as { text: string }).text;
    expect(text).toContain("Foo docs");
    expect(text).toContain("return 1");
    expect(text).not.toContain("func Bar");
  });

  it("read by display name fails with a concise error", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    await expect(call({ action: "read", path, symbol_id: "Foo" }, { cwd })).rejects.toThrow(
      /not found/,
    );
  });

  it("read renders Pi-equivalent continuation for truncated output", () => {
    const lines = Array.from({ length: 10 }, (_, i) => "line " + i);
    const text = renderReadText({
      path: "f",
      content: lines.slice(0, 3).join("\n"),
      start_line: 1,
      total_lines: 10,
      total_bytes: 100,
      truncated: true,
      truncated_by: "lines",
      output_lines: 3,
      next_offset: 4,
    });
    expect(text).toContain("[Showing lines 1-3 of 10. Use offset=4 to continue.]");
  });
});

describe("pi-src media reads", () => {
  it("returns an image note (and never decoded UTF-8 text) for a PNG file", async () => {
    const dir = mkdtempSync(join(tmpdir(), "pi-src-media-"));
    const path = join(dir, "img.png");
    // Minimal valid PNG header.
    const png = Buffer.concat([
      Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
      Buffer.from([0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52]),
    ]);
    writeFileSync(path, png);
    const result = await def.execute(
      "call-media",
      { action: "read", path } as any,
      undefined,
      undefined,
      { cwd: dir } as any,
    );
    const text = (result.content[0] as { text: string }).text;
    expect(text).toContain("Read image file");
  });

  it("adds a non-vision-model note when the current model cannot see images", async () => {
    const dir = mkdtempSync(join(tmpdir(), "pi-src-media-"));
    const path = join(dir, "img.png");
    const png = Buffer.concat([
      Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
      Buffer.from([0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52]),
    ]);
    writeFileSync(path, png);
    const result = await def.execute(
      "call-media",
      { action: "read", path } as any,
      undefined,
      undefined,
      { cwd: dir, model: { input: ["text"] } } as any,
    );
    const text = (result.content[0] as { text: string }).text;
    expect(text).toContain("[Current model does not support images.");
  });
});

describe("pi-src mutations", () => {
  it("replace rewrites the file and reports the diff", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const symbols = await call({ action: "symbols", path }, { cwd });
    const foo = (symbols.details as { symbols: Array<{ id: string; name: string }> }).symbols.find(
      (s) => s.name === "Foo",
    )!;
    const result = await call(
      { action: "replace", path, symbol_id: foo.id, content: "func Foo() {\n\treturn 99\n}" },
      { cwd },
    );
    const details = result.details as { action: string; symbol_id: string; diff: string };
    expect(details.action).toBe("replace");
    expect(details.symbol_id).toBe(foo.id);
    expect(details.diff).toContain("return 99");
    expect(readFileSync(path, "utf8")).toContain("return 99");
  });

  it("insert before/after uses the position flag", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const symbols = await call({ action: "symbols", path }, { cwd });
    const foo = (symbols.details as { symbols: Array<{ id: string; name: string }> }).symbols.find(
      (s) => s.name === "Foo",
    )!;
    await call(
      { action: "insert", path, symbol_id: foo.id, position: "after", content: "// after marker" },
      { cwd },
    );
    expect(readFileSync(path, "utf8")).toContain("// after marker");
  });

  it("delete removes the symbol", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const symbols = await call({ action: "symbols", path }, { cwd });
    const foo = (symbols.details as { symbols: Array<{ id: string; name: string }> }).symbols.find(
      (s) => s.name === "Foo",
    )!;
    await call({ action: "delete", path, symbol_id: foo.id }, { cwd });
    const after = readFileSync(path, "utf8");
    expect(after).not.toContain("func Foo");
    expect(after).toContain("func Bar");
  });

  it("comment read returns the doc comment; write replaces it", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const symbols = await call({ action: "symbols", path }, { cwd });
    const foo = (symbols.details as { symbols: Array<{ id: string; name: string }> }).symbols.find(
      (s) => s.name === "Foo",
    )!;
    const readResult = await call(
      { action: "comment", path, symbol_id: foo.id, read: true },
      { cwd },
    );
    expect((readResult.content[0] as { text: string }).text).toContain("Foo docs");
    await call({ action: "comment", path, symbol_id: foo.id, content: "// New docs." }, { cwd });
    expect(readFileSync(path, "utf8")).toContain("New docs.");
  });

  it("edit applies multiple disjoint replacements atomically", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const result = await call(
      {
        action: "edit",
        path,
        edits: [
          { oldText: "return 1", newText: "return 11" },
          { oldText: "return 2", newText: "return 22" },
        ],
      },
      { cwd },
    );
    const details = result.details as { edits_applied: number; diff: string };
    expect(details.edits_applied).toBe(2);
    const after = readFileSync(path, "utf8");
    expect(after).toContain("return 11");
    expect(after).toContain("return 22");
  });

  it("edit failure surfaces as a concise error", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    await expect(
      call({ action: "edit", path, edits: [{ oldText: "missing text", newText: "x" }] }, { cwd }),
    ).rejects.toThrow(/not found/);
  });
});

describe("pi-src registration", () => {
  it("registers exactly one global tool named src", async () => {
    const { registerSrcTool } = await import("../src/tool.js");
    const registered: Array<{ name: string }> = [];
    registerSrcTool({ registerTool: (d: any) => registered.push(d) } as any);
    expect(registered).toHaveLength(1);
    expect(registered[0]!.name).toBe("src");
  });
});
describe("pi-src takeover policy", () => {
  const builtinRead = {
    name: "read",
    description: "builtin read",
    parameters: {} as any,
    sourceInfo: { source: "builtin" } as any,
  };
  const builtinEdit = {
    name: "edit",
    description: "builtin edit",
    parameters: {} as any,
    sourceInfo: { source: "builtin" } as any,
  };
  const customTool = {
    name: "other",
    description: "extension tool",
    parameters: {} as any,
    sourceInfo: { source: "extension", path: "x" } as any,
  };

  function fakePi(active: string[], all: any[]) {
    let current = [...active];
    return {
      get current() {
        return [...current];
      },
      getActiveTools: () => [...current],
      getAllTools: () => all,
      setActiveTools: (names: string[]) => {
        current = [...names];
      },
    };
  }

  it("removes only active builtin read/edit, keeps other tools, activates src", () => {
    const pi = fakePi(["read", "edit", "bash", "other"], [builtinRead, builtinEdit, customTool]);
    const displaced = applyReadTakeover(pi);
    expect(displaced).toEqual(["read", "edit"]);
    expect(pi.current.sort()).toEqual(["bash", "other", "src"].sort());
  });

  it("always activates src even when no builtins are displaced", () => {
    const pi = fakePi(["bash"], [builtinRead, builtinEdit]);
    const displaced = applyReadTakeover(pi);
    expect(displaced).toEqual([]);
    expect(pi.current).toEqual(["bash", "src"]);
  });

  it("restores only the remembered subset at shutdown", () => {
    const pi = fakePi(["read", "edit", "bash"], [builtinRead, builtinEdit]);
    const displaced = applyReadTakeover(pi);
    // Another extension toggled a tool while active.
    pi.setActiveTools([...pi.current, "extra"]);
    restoreReadTakeover(pi, displaced);
    expect(pi.current.sort()).toEqual(["read", "edit", "bash", "extra", "src"].sort());
  });

  it("createTakeoverHandlers carries displaced state across events", () => {
    const pi = fakePi(["read", "edit"], [builtinRead, builtinEdit]);
    const handlers = createTakeoverHandlers(pi as any);
    handlers.onSessionStart();
    expect(pi.current).not.toContain("read");
    handlers.onSessionShutdown();
    expect(pi.current).toContain("read");
  });
});
