import type { TruncationResult } from "@earendil-works/pi-coding-agent";

import { truncateForModel, type ModelText, type TruncateForModelOptions } from "./truncate.js";

export interface TextContentBlock {
  type: "text";
  text: string;
}

/** Truncation metadata retained alongside structured tool details. */
export interface ModelTextDetails {
  truncation: TruncationResult;
  fullOutputPath: string;
}

/** A Pi text content block plus the complete structured domain result. */
export interface ModelTextToolResult<T extends object> {
  content: TextContentBlock[];
  details: T | (T & ModelTextDetails);
}

/**
 * Turns action-specific rendered text into a model-facing Pi result. Raw text
 * is bounded to Pi's standard limits, while pre-truncated text may supply its
 * own truncation metadata and full-content location (for example src reads).
 */
export async function modelTextResult<T extends object>(
  data: T,
  text: string | ModelText,
  options?: TruncateForModelOptions,
): Promise<ModelTextToolResult<T>> {
  const model = typeof text === "string" ? await truncateForModel(text, options) : text;
  if (!model.truncation) {
    return { content: [{ type: "text", text: model.text }], details: data };
  }
  if (!model.fullOutputPath) {
    throw new Error("truncated model text requires a full output path");
  }
  return {
    content: [{ type: "text", text: model.text }],
    details: {
      ...data,
      truncation: model.truncation,
      fullOutputPath: model.fullOutputPath,
    },
  };
}
