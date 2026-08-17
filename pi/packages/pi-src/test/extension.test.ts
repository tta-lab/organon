import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";

import {
  createEditToolDefinition,
  createReadToolDefinition,
  type EditToolDetails,
  type ReadToolDetails,
} from "@earendil-works/pi-coding-agent";
import { describe, expect, it } from "vitest";
import { Value } from "typebox/value";

import extension from "../src/index.js";
import { editSchema, editTool, readSchema, readTool, registerReadEditTools } from "../src/tool.js";
import { resolveSourcePath } from "../src/paths.js";

const readDefinition = readTool();
const editDefinition = editTool();

function makeFile(content: string, filename = "sample.go"): { path: string; cwd: string } {
  const dir = mkdtempSync(join(tmpdir(), "pi-src-"));
  const path = join(dir, filename);
  writeFileSync(path, content);
  return { path, cwd: dir };
}

function text(result: { content: Array<{ type: string; text?: string }> }): string {
  const block = result.content.find((item) => item.type === "text");
  return block?.text ?? "";
}

function detailsKeys(details: unknown): string[] {
  return Object.keys(details as Record<string, unknown>).sort();
}

const SAMPLE =
  "package sample\n\n// Foo docs.\nfunc Foo() {\n\treturn 1\n}\n\nfunc Bar() {\n\treturn 2\n}\n";

async function callRead(params: unknown, cwd: string) {
  return readDefinition.execute("read-call", params as any, undefined, undefined, { cwd } as any);
}

async function callEdit(params: unknown, cwd: string) {
  return editDefinition.execute("edit-call", params as any, undefined, undefined, { cwd } as any);
}

describe("pi-src override schemas", () => {
  it("accepts the live built-in branches and the closed Organon forms", () => {
    const builtInRead = createReadToolDefinition(process.cwd());
    const builtInEdit = createEditToolDefinition(process.cwd());

    for (const input of [
      { path: "a.go" },
      { path: "a.go", offset: 2, limit: 10 },
      { path: "a.go", limit: 0 },
    ]) {
      expect(Value.Check(builtInRead.parameters, input)).toBe(true);
      expect(Value.Check(readSchema, input)).toBe(true);
    }
    const exactEdit = { path: "a.go", edits: [{ oldText: "a", newText: "b" }] };
    expect(Value.Check(builtInEdit.parameters, exactEdit)).toBe(true);
    expect(Value.Check(editSchema, exactEdit)).toBe(true);

    expect(Value.Check(readSchema, { path: "a.go", symbols: true })).toBe(true);
    expect(Value.Check(readSchema, { path: "a.go", symbol_id: "bK", offset: 1, limit: 10 })).toBe(
      true,
    );
    expect(
      Value.Check(editSchema, {
        path: "a.go",
        operation: "replace",
        symbol_id: "bK",
        content: "x",
      }),
    ).toBe(true);
    expect(
      Value.Check(editSchema, {
        path: "a.go",
        operation: "insert",
        symbol_id: "bK",
        position: "before",
        content: "x",
      }),
    ).toBe(true);
    expect(Value.Check(editSchema, { path: "a.go", operation: "delete", symbol_id: "bK" })).toBe(
      true,
    );
    expect(
      Value.Check(editSchema, {
        path: "a.go",
        operation: "comment",
        symbol_id: "bK",
        content: "docs",
      }),
    ).toBe(true);
  });

  it("uses direct superset schemas and rejects incompatible modes at execution", async () => {
    expect(Value.Check(readSchema, { path: "a.go", symbols: true, offset: 1 })).toBe(true);
    expect(Value.Check(editSchema, { path: "a.go", operation: "replace", symbol_id: "bK" })).toBe(
      true,
    );
    expect(Value.Check(readSchema, { path: "a.go", symbols: false })).toBe(false);
    expect(Value.Check(readSchema, { path: "a.go", symbol_id: "" })).toBe(false);
    expect(Value.Check(readSchema, { path: "a.go", symbols: true, bogus: true })).toBe(false);
    expect(Value.Check(editSchema, { path: "a.go", operation: "nope" })).toBe(false);
    expect(Value.Check(editSchema, { path: "a.go", operation: "replace", extra: true })).toBe(
      false,
    );

    const { path, cwd } = makeFile(SAMPLE);
    await expect(callRead({ path, symbols: true, offset: 1 }, cwd)).rejects.toThrow(
      /does not accept offset or limit/,
    );
    await expect(callRead({ path, symbol_id: "bK", symbols: true }, cwd)).rejects.toThrow(
      /combine symbols and symbol_id/,
    );
    await expect(callEdit({ path, operation: "replace", symbol_id: "bK" }, cwd)).rejects.toThrow(
      /replace requires content/,
    );
    await expect(
      callEdit({ path, operation: "insert", symbol_id: "bK", content: "x" }, cwd),
    ).rejects.toThrow(/requires position/);
    await expect(
      callEdit({ path, edits: [], operation: "delete", symbol_id: "bK" }, cwd),
    ).rejects.toThrow(/combine operation with edits/);
  });

  it("keeps built-in edit argument preparation before validation", () => {
    const prepared = editDefinition.prepareArguments?.({
      path: "a.go",
      oldText: "old",
      newText: "new",
    });
    expect(prepared).toEqual({ path: "a.go", edits: [{ oldText: "old", newText: "new" }] });
    expect(Value.Check(editSchema, prepared)).toBe(true);

    const operation = { path: "a.go", operation: "delete", symbol_id: "bK" };
    expect(editDefinition.prepareArguments?.(operation)).toBe(operation);
  });
});

describe("pi-src registration", () => {
  it("registers exactly read and edit without changing active tools", () => {
    const registered: Array<{ name: string }> = [];
    const lifecycleEvents: string[] = [];
    registerReadEditTools({
      registerTool: (definition: any) => registered.push(definition),
    });
    expect(registered.map((definition) => definition.name)).toEqual(["read", "edit"]);

    extension({
      registerTool: () => undefined,
      on: (event: string) => lifecycleEvents.push(event),
    } as any);
    expect(lifecycleEvents).toEqual([]);
  });
});

describe("pi-src reads", () => {
  it("resolves relative, absolute, leading-at, and whitespace-containing paths", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    expect(resolveSourcePath("@" + path, cwd)).toBe(path);
    expect(resolveSourcePath(path, cwd)).toBe(path);
    expect(resolveSourcePath("sample.go", cwd)).toBe(path);

    for (const relative of [" leading.go", "trailing.go "]) {
      const whitespacePath = join(cwd, relative);
      writeFileSync(whitespacePath, SAMPLE);
      expect(text(await callRead({ path: relative }, cwd))).toContain("func Foo");
    }
  });

  it("keeps ordinary whole-file and offset/limit reads on the built-in result shape", async () => {
    const { path, cwd } = makeFile(
      "module example.com/organon\n" +
        "go 1.26\n" +
        "require example.com/dependency v1.0.0\n" +
        "replace example.com/dependency => ./dependency\n",
      "go.mod",
    );
    const whole = await callRead({ path }, cwd);
    expect(text(whole)).toContain("module example.com/organon");
    expect(whole.details).toBeUndefined();

    const window = await callRead({ path, offset: 2, limit: 2 }, cwd);
    expect(text(window)).toContain("go 1.26");
    expect(text(window)).toContain("require example.com/dependency");
    expect(text(window)).not.toContain("module example.com/organon");
    expect(text(window)).toContain("more lines in file");
    expect(window.details).toBeUndefined();
  });

  it("preserves an explicit zero limit as an empty selection with continuation", async () => {
    const { path, cwd } = makeFile("one\ntwo\nthree\n", "window.txt");
    const result = await callRead({ path, offset: 2, limit: 0 }, cwd);

    expect(text(result)).not.toContain("two");
    expect(text(result)).not.toContain("three");
    expect(text(result)).toContain("[3 more lines in file. Use offset=2 to continue.");
    expect(result.details).toBeUndefined();
  });

  it("returns an outline with display names and opaque IDs, then reads one exact symbol", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const outline = await callRead({ path, symbols: true }, cwd);
    const outlineText = text(outline);
    expect(outlineText).toContain("Foo");
    expect(outlineText).toContain("Bar");
    expect(outlineText).toMatch(/\[[^\]]+\] function Foo/);
    expect(outline.details).toBeUndefined();

    const id = outlineText.match(/- \[([^\]]+)\] function Foo/)?.[1];
    expect(id).toBeTruthy();
    const selected = await callRead({ path, symbol_id: id }, cwd);
    expect(text(selected)).toContain("Foo docs");
    expect(text(selected)).toContain("return 1");
    expect(text(selected)).not.toContain("func Bar");
    expect(selected.details).toBeUndefined();
  });

  it("rejects display names and explicit empty symbol IDs", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    await expect(callRead({ path, symbol_id: "Foo" }, cwd)).rejects.toThrow(/not found/);
    await expect(callRead({ path, symbol_id: "" }, cwd)).rejects.toThrow(
      "symbol_id must not be empty",
    );
  });

  it("paginates relative to a selected symbol", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const outline = await callRead({ path, symbols: true }, cwd);
    const id = text(outline).match(/- \[([^\]]+)\] function Foo/)?.[1];
    const selected = await callRead({ path, symbol_id: id, offset: 3, limit: 1 }, cwd);
    expect(text(selected)).toContain("return 1");
    expect(text(selected)).not.toContain("Foo docs");
    expect(selected.details).toBeUndefined();
  });

  it("keeps outline truncation metadata compatible with ReadToolDetails", async () => {
    const { path, cwd } = makeFile(
      "package sample\n\n" +
        Array.from({ length: 2200 }, (_, index) => `func F${index}() {\n}`).join("\n"),
    );
    const result = await callRead({ path, symbols: true }, cwd);
    const details = result.details as ReadToolDetails;
    expect(details.truncation?.truncated).toBe(true);
    expect(details).not.toHaveProperty("fullOutputPath");
    const fullOutputPath = text(result).match(/Full output saved to: (.+)\]/)?.[1];
    expect(fullOutputPath).toBeTruthy();
    rmSync(dirname(fullOutputPath!), { recursive: true, force: true });
  });
});

describe("pi-src media reads", () => {
  function onePixelBMP(): Buffer {
    const bmp = Buffer.alloc(58);
    bmp.write("BM");
    bmp.writeUInt32LE(58, 2);
    bmp.writeUInt32LE(54, 10);
    bmp.writeUInt32LE(40, 14);
    bmp.writeInt32LE(1, 18);
    bmp.writeInt32LE(1, 22);
    bmp.writeUInt16LE(1, 26);
    bmp.writeUInt16LE(24, 28);
    bmp.writeUInt32LE(4, 34);
    bmp.set([0, 0, 255, 0], 54);
    return bmp;
  }

  it("returns image content without custom details and adds the model note", async () => {
    const { cwd } = makeFile("", "img.png");
    const path = join(cwd, "img.png");
    writeFileSync(
      path,
      Buffer.concat([
        Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
        Buffer.from([0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52]),
      ]),
    );
    const result = await readDefinition.execute("media", { path } as any, undefined, undefined, {
      cwd,
      model: { input: ["text"] },
    } as any);
    expect(text(result)).toContain("Read image file");
    expect(text(result)).toContain("Current model does not support images");
    expect(result.details).toBeUndefined();
  });

  it("normalizes BMP to an inline PNG", async () => {
    const { cwd } = makeFile("", "img.bmp");
    const path = join(cwd, "img.bmp");
    writeFileSync(path, onePixelBMP());
    const result = await readDefinition.execute("media", { path } as any, undefined, undefined, {
      cwd,
    } as any);
    const image = result.content.find(
      (block): block is { type: "image"; data: string; mimeType: string } => block.type === "image",
    );
    expect(image?.mimeType).toBe("image/png");
    expect(Buffer.from(image!.data, "base64").subarray(0, 8)).toEqual(
      Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    );
  });
});

describe("pi-src edits", () => {
  it("applies exact multi-edit batches without symbol discovery", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const result = await callEdit(
      {
        path,
        edits: [
          { oldText: "return 1", newText: "return 11" },
          { oldText: "return 2", newText: "return 22" },
        ],
      },
      cwd,
    );
    const details = result.details as EditToolDetails;
    expect(detailsKeys(details)).toEqual(["diff", "firstChangedLine", "patch"]);
    expect(details.diff).toContain("return 11");
    expect(details.patch).toContain("--- a/");
    expect(details.patch).toContain("@@");
    expect(text(result)).toBe(`Successfully replaced 2 block(s) in ${path}.`);
    expect(readFileSync(path, "utf8")).toContain("return 22");
  });

  it("surfaces exact edit failures without writing the file", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    await expect(
      callEdit({ path, edits: [{ oldText: "missing text", newText: "x" }] }, cwd),
    ).rejects.toThrow(/not found/);
    expect(readFileSync(path, "utf8")).toBe(SAMPLE);
  });

  it("continues all symbol operations from each returned post-edit outline", async () => {
    const { path, cwd } = makeFile(SAMPLE);
    const initial = text(await callRead({ path, symbols: true }, cwd));
    const foo = initial.match(/- \[([^\]]+)\] function Foo/)![1]!;
    let result = await callEdit(
      { path, operation: "replace", symbol_id: foo, content: "func Foo() {\n\treturn 99\n}" },
      cwd,
    );
    expect(text(result)).toContain(`Applied replace ${foo} to ${path}.`);
    expect(text(result)).toContain("Post-edit outline:");
    expect(text(result)).toContain("function Foo");
    expect(detailsKeys(result.details)).toEqual(["diff", "firstChangedLine", "patch"]);
    expect((result.details as EditToolDetails).patch).toContain("--- a/");
    expect(readFileSync(path, "utf8")).toContain("return 99");

    let current = text(result);
    const bar = current.match(/- \[([^\]]+)\] function Bar/)![1]!;
    result = await callEdit(
      {
        path,
        operation: "insert",
        symbol_id: bar,
        position: "before",
        content: "func Before() {}",
      },
      cwd,
    );
    expect(text(result)).toContain("function Before");
    expect((result.details as EditToolDetails).diff).toContain("Before");

    current = text(result);
    const before = current.match(/- \[([^\]]+)\] function Before/)![1]!;
    result = await callEdit(
      { path, operation: "comment", symbol_id: before, content: "Before docs." },
      cwd,
    );
    expect(text(result)).toContain("function Before");
    expect((result.details as EditToolDetails).firstChangedLine).toBe(1);
    expect(readFileSync(path, "utf8")).toContain("Before docs.");

    current = text(result);
    const barAfterInsert = current.match(/- \[([^\]]+)\] function Bar/)![1]!;
    result = await callEdit({ path, operation: "delete", symbol_id: barAfterInsert }, cwd);
    expect(text(result)).not.toContain("function Bar");
    expect((result.details as EditToolDetails).patch).toContain("--- a/");
    expect(readFileSync(path, "utf8")).not.toContain("func Bar");
  });

  it("reads and replaces a targetable Markdown H1 section", async () => {
    const { path, cwd } = makeFile("# Guide\n\n## Setup\n\nInstall it.\n", "guide.md");
    const initial = text(await callRead({ path, symbols: true }, cwd));
    const guide = initial.match(/- \[([^\]]+)\] section Guide/)![1]!;

    const selected = await callRead({ path, symbol_id: guide }, cwd);
    expect(text(selected)).toContain("# Guide");
    expect(text(selected)).toContain("## Setup");

    const result = await callEdit(
      { path, operation: "replace", symbol_id: guide, content: "# New Guide\n\nNew body.\n" },
      cwd,
    );
    expect(text(result)).toContain("section New Guide");
    expect(readFileSync(path, "utf8")).toBe("# New Guide\n\nNew body.");
  });

  it("returns a typed post-edit outline for Markdown heading sections", async () => {
    const { path, cwd } = makeFile(
      "# Guide\n\n## Setup\n\nInstall it.\n\n## Other\n\nKeep this.\n",
      "guide.md",
    );
    const initial = text(await callRead({ path, symbols: true }, cwd));
    const setup = initial.match(/- \[([^\]]+)\] section Setup/)![1]!;
    const result = await callEdit(
      { path, operation: "replace", symbol_id: setup, content: "## Setup\n\nUse it." },
      cwd,
    );

    expect(text(result)).toContain("Post-edit outline:");
    expect(text(result)).toContain("(markdown)");
    expect(text(result)).toContain("section Setup");
    expect((result.details as EditToolDetails).diff).toContain("Use it.");
  });

  it("reports an empty post-edit outline when the last symbol is deleted", async () => {
    const { path, cwd } = makeFile("package sample\n\nfunc Foo() {\n}\n");
    const initial = text(await callRead({ path, symbols: true }, cwd));
    const foo = initial.match(/- \[([^\]]+)\] function Foo/)![1]!;
    const result = await callEdit({ path, operation: "delete", symbol_id: foo }, cwd);

    expect(text(result)).toContain("Applied delete");
    expect(text(result)).toContain("Post-edit outline:");
    expect(text(result)).toContain("No symbols found.");
    expect((result.details as EditToolDetails).patch).toContain("--- a/");
  });

  it("keeps mutation confirmation visible when the returned outline is truncated", async () => {
    const { path, cwd } = makeFile(
      "package sample\n\n" +
        Array.from({ length: 2200 }, (_, index) => `func F${index}() {\n}\n`).join("\n"),
    );
    const result = await callEdit(
      { path, operation: "replace", symbol_id: "0", content: "func F0() {\n}\n" },
      cwd,
    );
    const output = text(result);
    const fullOutputPath = output.match(/Full output saved to: (.+)\]/)?.[1];

    try {
      expect(output).toContain("Applied replace 0");
      expect(output).toContain("Post-edit outline:");
      expect(output).toContain("Truncated");
      expect(output).toContain("needed entry was omitted");
      expect(fullOutputPath).toBeTruthy();
      expect(detailsKeys(result.details)).toEqual(["diff", "firstChangedLine", "patch"]);
    } finally {
      if (fullOutputPath) rmSync(dirname(fullOutputPath), { recursive: true, force: true });
    }
  });

  it("rejects the removed comment-read form", () => {
    expect(
      Value.Check(editSchema, { path: "a.go", operation: "comment", symbol_id: "bK", read: true }),
    ).toBe(false);
  });
});
