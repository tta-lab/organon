import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { editTool, readTool } from "../src/tool.js";

const readDefinition = readTool();
const editDefinition = editTool();

async function runEdit(path: string, cwd: string, oldText: string, newText: string) {
  return editDefinition.execute(
    "call-q",
    { path, edits: [{ oldText, newText }] } as any,
    undefined,
    undefined,
    { cwd } as any,
  );
}

async function runSymbolEdit(path: string, cwd: string, symbol_id: string, content: string) {
  return editDefinition.execute(
    "call-q-symbol",
    { path, operation: "replace", symbol_id, content } as any,
    undefined,
    undefined,
    { cwd } as any,
  );
}

function idsByName(result: {
  content: Array<{ type: string; text?: string }>;
}): Record<string, string> {
  const outline = result.content.find((block) => block.type === "text")?.text ?? "";
  return Object.fromEntries(
    [...outline.matchAll(/- \[([^\]]+)\] function (\w+)/g)].map((match) => [match[2]!, match[1]!]),
  );
}

// Observable behavior test: concurrent mutations to the same file must
// serialize through Pi's per-file mutation queue so that every edit survives
// the child-process read-modify-write window. Without the queue, two parallel
// spawns would read the same snapshot and one write would clobber the other.
describe("pi-src mutation serialization", () => {
  it("applies concurrent exact edits without losing changes", async () => {
    const dir = mkdtempSync(join(tmpdir(), "pi-src-queue-"));
    const path = join(dir, "sample.go");
    writeFileSync(
      path,
      "package sample\n\nfunc Foo() {\n\treturn 1\n}\n\nfunc Bar() {\n\treturn 2\n}\n",
    );

    await Promise.all([
      runEdit(path, dir, "return 1", "return 11"),
      runEdit(path, dir, "return 2", "return 22"),
    ]);

    const after = readFileSync(path, "utf8");
    expect(after).toContain("return 11");
    expect(after).toContain("return 22");
  });

  it("serializes concurrent symbol edits through the same file queue", async () => {
    const dir = mkdtempSync(join(tmpdir(), "pi-src-queue-symbol-"));
    const path = join(dir, "sample.go");
    writeFileSync(
      path,
      "package sample\n\nfunc Foo() {\n\treturn 1\n}\n\nfunc Bar() {\n\treturn 2\n}\n",
    );
    const outline = await readDefinition.execute(
      "call-outline",
      { path, symbols: true } as any,
      undefined,
      undefined,
      { cwd: dir } as any,
    );
    const ids = idsByName(outline);

    const results = await Promise.all([
      runSymbolEdit(path, dir, ids.Foo!, "func Foo() {\n\treturn 11\n}"),
      runSymbolEdit(path, dir, ids.Bar!, "func Bar() {\n\treturn 22\n}"),
    ]);

    const after = readFileSync(path, "utf8");
    expect(after).toContain("return 11");
    expect(after).toContain("return 22");
    expect(results).toHaveLength(2);
    expect(
      results.every((result) => (result.details as { patch: string }).patch.includes("--- a/")),
    ).toBe(true);
  });
});
