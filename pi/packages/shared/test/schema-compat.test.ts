import { describe, expect, it } from "vitest";
import type { TSchema } from "typebox";

import {
  ogChecksSchema,
  ogCloneSchema,
  ogPrSchema,
  ogPullSchema,
  ogPushSchema,
} from "../../pi-og/src/tool.js";
import {
  projectFindSchema,
  projectGetSchema,
  projectListSchema,
} from "../../pi-project/src/tool.js";
import { editSchema, readSchema } from "../../pi-src/src/tool.js";

interface JsonSchemaLike {
  type?: unknown;
  const?: unknown;
  properties?: Record<string, JsonSchemaLike>;
  anyOf?: JsonSchemaLike[];
  oneOf?: JsonSchemaLike[];
}

function constPaths(value: unknown, path = "$", seen = new Set<object>()): string[] {
  if (value === null || typeof value !== "object" || seen.has(value as object)) return [];
  seen.add(value as object);
  if (Array.isArray(value)) {
    return value.flatMap((item, index) => constPaths(item, `${path}[${index}]`, seen));
  }
  const record = value as Record<string, unknown>;
  const paths = Object.prototype.hasOwnProperty.call(record, "const") ? [path] : [];
  for (const [key, child] of Object.entries(record))
    paths.push(...constPaths(child, `${path}.${key}`, seen));
  return paths;
}

describe("registered Pi tool schemas are provider-compatible", () => {
  const tools: Array<[string, TSchema]> = [
    ["read", readSchema],
    ["edit", editSchema],
    ["project_list", projectListSchema],
    ["project_find", projectFindSchema],
    ["project_get", projectGetSchema],
    ["og_clone", ogCloneSchema],
    ["og_pull", ogPullSchema],
    ["og_push", ogPushSchema],
    ["og_pr", ogPrSchema],
    ["og_checks", ogChecksSchema],
  ];

  it.each(tools)("%s is a closed direct object schema", (name, schema) => {
    const root = schema as unknown as JsonSchemaLike;
    expect(root.type, `${name}: root type`).toBe("object");
    expect(root.anyOf, `${name}: root anyOf`).toBeUndefined();
    expect(root.oneOf, `${name}: root oneOf`).toBeUndefined();
  });

  it.each(tools)("%s contains no const fields", (name, schema) => {
    expect(constPaths(schema), `${name}: Type.Literal/const is not Google-compatible`).toEqual([]);
  });
});
