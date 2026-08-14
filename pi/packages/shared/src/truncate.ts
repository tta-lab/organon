import {
  DEFAULT_MAX_BYTES,
  formatSize,
  truncateHead,
  type TruncationResult,
} from "@earendil-works/pi-coding-agent";

export interface ModelText {
  text: string;
  truncation?: TruncationResult;
}

/**
 * Truncates model-facing text per Pi's 2,000-line / 50-KB contract and appends
 * an actionable notice. The full structured result stays in tool-result
 * details. Every tool's model-facing content must pass through this so no
 * extension can return unbounded output.
 */
export function truncateForModel(content: string, options?: { hint?: string }): ModelText {
  const truncation = truncateHead(content);
  if (!truncation.truncated) {
    return { text: content };
  }
  const hint = options?.hint ? `. ${options.hint}` : "";
  let text: string;
  if (truncation.firstLineExceedsLimit) {
    text = `[First line is ${formatSize(truncation.totalBytes)}, exceeds ${formatSize(DEFAULT_MAX_BYTES)} limit]`;
  } else if (truncation.truncatedBy === "lines") {
    text =
      truncation.content +
      `\n\n[Truncated: showing ${truncation.outputLines} of ${truncation.totalLines} lines ` +
      `(${truncation.maxLines} line limit)${hint}]`;
  } else {
    text =
      truncation.content +
      `\n\n[Truncated: ${truncation.outputLines} lines shown (${formatSize(truncation.maxBytes)} limit)${hint}]`;
  }
  return { text, truncation };
}
