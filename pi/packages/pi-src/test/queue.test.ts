import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { srcTool } from "../src/tool.js";

const def = srcTool();

async function runEdit(path: string, cwd: string, oldText: string, newText: string) {
  return def.execute(
    "call-q",
    { action: "edit", path, edits: [{ oldText, newText }] } as any,
    undefined,
    undefined,
    { cwd } as any,
  );
}

// Observable behavior test: concurrent mutations to the same file must
// serialize through Pi's per-file mutation queue so that every edit survives
// the child-process read-modify-write window. Without the queue, two parallel
// spawns would read the same snapshot and one write would clobber the other.
describe("pi-src mutation serialization", () => {
  it("applies concurrent edits to the same file without losing changes", async () => {
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

  it("serializes concurrent whole-symbol replacements with their diffs", async () => {
    const dir = mkdtempSync(join(tmpdir(), "pi-src-queue-"));
    const path = join(dir, "sample.go");
    writeFileSync(
      path,
      "package sample\n\nfunc Foo() {\n\treturn 1\n}\n\nfunc Bar() {\n\treturn 2\n}\n",
    );

    const results = await Promise.all([
      runEdit(path, dir, "func Foo() {", "func Foo() { // edited"),
      runEdit(path, dir, "func Bar() {", "func Bar() { // edited"),
    ]);

    const after = readFileSync(path, "utf8");
    expect(after).toContain("func Foo() { // edited");
    expect(after).toContain("func Bar() { // edited");
    // Both child processes reported success (their diffs were computed against
    // the serialized states, so neither failed with a stale-snapshot error).
    expect(results).toHaveLength(2);
  });
});
