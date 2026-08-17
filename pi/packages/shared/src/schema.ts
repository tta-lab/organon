import { Type, type TSchema, type TUnion } from "typebox";

/**
 * Creates a TypeBox union that remains an object schema at provider boundaries.
 * Some providers reject a root anyOf without an explicit JSON Schema type.
 */
export function objectUnion<Types extends TSchema[]>(schemas: [...Types]): TUnion<Types> {
  return Type.Union(schemas, { type: "object" });
}
