import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  DEFAULT_MAX_BYTES,
  formatSize,
  truncateHead,
  type TruncationResult,
} from "@earendil-works/pi-coding-agent";

export interface ModelText {
  text: string;
  truncation?: TruncationResult;
  fullOutputPath?: string;
}

export interface TruncateForModelOptions {
  /** Action-specific guidance for retrieving or narrowing the remaining output. */
  hint?: string;
}

/**
 * Truncates model-facing text per Pi's 2,000-line / 50-KB contract. When text
 * is truncated, saves the complete rendered output to a private temporary file
 * and tells the model where to find it.
 */
export async function truncateForModel(
  content: string,
  options?: TruncateForModelOptions,
): Promise<ModelText> {
  const truncation = truncateHead(content);
  if (!truncation.truncated) {
    return { text: content };
  }

  const fullOutputPath = await saveFullOutput(content);
  const hint = options?.hint?.trim() ? ` ${options.hint.trim()}` : "";
  let text: string;
  if (truncation.firstLineExceedsLimit) {
    text =
      `[First line is ${formatSize(truncation.totalBytes)}, exceeds ${formatSize(DEFAULT_MAX_BYTES)} limit.` +
      `${hint} Full output saved to: ${fullOutputPath}]`;
  } else if (truncation.truncatedBy === "lines") {
    text =
      truncation.content +
      `\n\n[Truncated: showing ${truncation.outputLines} of ${truncation.totalLines} lines ` +
      `(${truncation.maxLines} line limit).${hint} Full output saved to: ${fullOutputPath}]`;
  } else {
    text =
      truncation.content +
      `\n\n[Truncated: ${truncation.outputLines} lines shown (${formatSize(truncation.maxBytes)} limit).` +
      `${hint} Full output saved to: ${fullOutputPath}]`;
  }
  return { text, truncation, fullOutputPath };
}

async function saveFullOutput(content: string): Promise<string> {
  const directory = await mkdtemp(join(tmpdir(), "pi-tool-output-"));
  const path = join(directory, "output.txt");
  await writeFile(path, content, { encoding: "utf8", mode: 0o600 });
  return path;
}
