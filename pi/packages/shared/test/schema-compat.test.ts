import { describe, expect, it } from "vitest";
import type { TSchema } from "typebox";

import { ogSchema } from "../../pi-og/src/tool.js";
import { projectSchema } from "../../pi-project/src/tool.js";
import { srcSchema } from "../../pi-src/src/tool.js";
import { webSchema } from "../../pi-web/src/tool.js";

interface JsonSchemaLike {
  const?: unknown;
  enum?: unknown[];
  properties?: Record<string, JsonSchemaLike>;
  anyOf?: JsonSchemaLike[];
  oneOf?: JsonSchemaLike[];
}

function memberSchemas(schema: TSchema): JsonSchemaLike[] {
  const raw = schema as unknown as JsonSchemaLike;
  return [...(raw.anyOf ?? []), ...(raw.oneOf ?? [])];
}

// Every action discriminator must be expressed as a string enum, never as a
// const/Type.Literal, because Pi's docs require StringEnum for Google-compatible
// tool schemas (some Google/OpenAPI conversion paths cannot express const).
describe("action discriminators are Google-compatible string enums", () => {
  const tools: Array<[string, TSchema]> = [
    ["src", srcSchema],
    ["web", webSchema],
    ["project", projectSchema],
    ["og", ogSchema],
  ];

  it.each(tools)("%s: every union member's action field uses enum, not const", (name, schema) => {
    const members = memberSchemas(schema);
    expect(members.length).toBeGreaterThan(0);
    for (const member of members) {
      const action = member.properties?.action;
      expect(action, `${name}: member is missing the action property`).toBeDefined();
      expect(action!.const, `${name}: action must not use const`).toBeUndefined();
      expect(Array.isArray(action!.enum), `${name}: action must be a string enum`).toBe(true);
      expect(action!.enum!.length).toBe(1);
    }
  });
});
