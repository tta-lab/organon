import { describe, expect, it } from "vitest";
import type { TSchema } from "typebox";

import {
  ogAuthStatusSchema,
  ogChecksSchema,
  ogCloneSchema,
  ogPrSchema,
  ogPullSchema,
  ogPushSchema,
} from "../../pi-og/src/tool.js";
import { projectSchema } from "../../pi-project/src/tool.js";
import { editSchema, readSchema } from "../../pi-src/src/tool.js";
import { webSchema } from "../../pi-web/src/tool.js";

interface JsonSchemaLike {
  type?: unknown;
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

function constPaths(value: unknown, path = "$", seen = new Set<object>()): string[] {
  if (value === null || typeof value !== "object") {
    return [];
  }
  if (seen.has(value)) {
    return [];
  }
  seen.add(value);
  if (Array.isArray(value)) {
    return value.flatMap((item, index) => constPaths(item, `${path}[${index}]`, seen));
  }

  const record = value as Record<string, unknown>;
  const paths = Object.prototype.hasOwnProperty.call(record, "const") ? [path] : [];
  for (const [key, child] of Object.entries(record)) {
    paths.push(...constPaths(child, `${path}.${key}`, seen));
  }
  return paths;
}

// Pi's docs prohibit const/Type.Literal anywhere in a tool schema because
// Google's API cannot express those structures. Organon uses enum-backed
// booleans and StringEnum discriminators instead.
describe("tool schemas are Google-compatible", () => {
  const tools: Array<[string, TSchema]> = [
    ["read", readSchema],
    ["edit", editSchema],
    ["web", webSchema],
    ["project", projectSchema],
    ["og_auth_status", ogAuthStatusSchema],
    ["og_clone", ogCloneSchema],
    ["og_pull", ogPullSchema],
    ["og_push", ogPushSchema],
    ["og_pr", ogPrSchema],
    ["og_checks", ogChecksSchema],
  ];

  const groupedTools: Array<[string, TSchema]> = [
    ["og_pr", ogPrSchema],
    ["og_checks", ogChecksSchema],
  ];

  it.each(tools)("%s: exposes an object schema at the provider boundary", (name, schema) => {
    const root = schema as unknown as JsonSchemaLike;
    expect(root.type, `${name}: provider tool schemas must have type object`).toBe("object");
  });

  it.each(tools)("%s: contains no const fields at any schema depth", (name, schema) => {
    expect(constPaths(schema), `${name}: Type.Literal/const is not Google-compatible`).toEqual([]);
  });

  it.each(groupedTools)(
    "%s: every union member's action field uses a string enum",
    (name, schema) => {
      const members = memberSchemas(schema);
      expect(members.length).toBeGreaterThan(0);
      for (const member of members) {
        const action = member.properties?.action;
        expect(action, `${name}: member is missing the action property`).toBeDefined();
        expect(action!.const, `${name}: action must not use const`).toBeUndefined();
        expect(Array.isArray(action!.enum), `${name}: action must be a string enum`).toBe(true);
        expect(action!.enum!.length).toBe(1);
      }
    },
  );
});
