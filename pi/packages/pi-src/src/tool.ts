import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import {
  convertToPng,
  formatDimensionNote,
  formatSize,
  resizeImage,
  type TruncationResult,
  withFileMutationQueue,
} from "@earendil-works/pi-coding-agent";
import { Type, type Static } from "typebox";

import {
  cliError,
  parseSingleJsonDoc,
  resolveBinaryPath,
  runCli,
  truncateForModel,
} from "@tta-lab/pi-shared";

import { resolveSourcePath } from "./paths.js";

const require = createRequire(import.meta.url);

const symbolIdDescription =
  "Exact opaque symbol or Markdown section ID returned by src action symbols; never a display name.";

const pathDescription = "Path to the file (absolute, or relative to the current working directory)";

const editEntry = Type.Object(
  {
    oldText: Type.String({ description: "Exact original text to replace (may be multiline)" }),
    newText: Type.String({ description: "Replacement text (may be multiline)" }),
  },
  { additionalProperties: false },
);

export const srcSchema = Type.Union([
  Type.Object(
    {
      action: StringEnum(["symbols"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["read"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
      symbol_id: Type.Optional(Type.String({ description: symbolIdDescription })),
      offset: Type.Optional(
        Type.Integer({
          description: "1-indexed line offset within the selected content",
          minimum: 1,
        }),
      ),
      limit: Type.Optional(
        Type.Integer({ description: "Maximum number of lines to read", minimum: 1 }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["replace"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
      symbol_id: Type.String({ description: symbolIdDescription }),
      content: Type.String({
        description: "New content of the symbol or Markdown section (may be multiline)",
      }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["insert"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
      symbol_id: Type.String({ description: symbolIdDescription }),
      position: StringEnum(["before", "after"] as const, {
        description: "Insert before or after the symbol",
      }),
      content: Type.String({ description: "Content to insert (may be multiline)" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["delete"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
      symbol_id: Type.String({ description: symbolIdDescription }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["comment"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
      symbol_id: Type.String({ description: symbolIdDescription }),
      read: Type.Literal(true, { description: "Read the existing doc comment" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["comment"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
      symbol_id: Type.String({ description: symbolIdDescription }),
      content: Type.String({ description: "New doc comment (may be multiline)" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["edit"] as const, { description: "Action to perform" }),
      path: Type.String({ description: pathDescription }),
      edits: Type.Array(editEntry, {
        description:
          "Exact text replacements; every oldText must match one unique region of the original file and entries must not overlap",
        minItems: 1,
      }),
    },
    { additionalProperties: false },
  ),
]);

export type SrcInput = Static<typeof srcSchema>;

export interface SymbolsResult {
  path: string;
  language: string;
  title?: string;
  total_bytes: number;
  symbols: Array<{
    id: string;
    targetable: boolean;
    name: string;
    kind: string;
    parent?: string;
    level: number;
    start_byte: number;
    end_byte: number;
    start_line: number;
    end_line: number;
    has_doc: boolean;
  }>;
}

export interface ReadResult {
  path: string;
  symbol_id?: string;
  content: string;
  start_line: number;
  total_lines: number;
  total_bytes: number;
  truncated: boolean;
  truncated_by?: string;
  output_lines?: number;
  output_bytes?: number;
  next_offset?: number;
  first_line_exceeds_limit?: boolean;
  media?: { kind: string; mime: string; data_base64: string };
}

export interface MutationResult {
  path: string;
  action: string;
  symbol_id?: string;
  diff: string;
  first_changed_line?: number;
}

export interface CommentReadResult {
  path: string;
  symbol_id: string;
  comment: string;
}

export interface EditBatchResult {
  path: string;
  diff: string;
  patch: string;
  first_changed_line?: number;
  edits_applied: number;
}

const SRC_PROMPT_GUIDELINES = [
  "Prefer src symbol-aware operations for structured source and Markdown files because they usually require less content and are more efficient.",
  "Before a symbol-scoped src read or mutation, call src with action symbols for the current file and copy the exact returned ID.",
  "A src symbol ID is the opaque ID returned by src action symbols; it is not the displayed symbol name and must never be guessed or constructed from a function, class, method, variable, or section name.",
  "src action edit does not require symbols when the exact original text is already known.",
  "When exact text is not already known, prefer src action symbols followed by a symbol-aware read or mutation.",
  "Refresh src symbols after a structural edit before another symbol-scoped operation because IDs may have changed.",
  "For multiple disjoint exact replacements in one file, use one src action edit with multiple entries. Entries match the original file and must not overlap.",
];

type Action =
  | { action: "symbols"; path: string }
  | {
      action: "read";
      path: string;
      symbol_id?: string;
      offset?: number;
      limit?: number;
    }
  | { action: "replace"; path: string; symbol_id: string; content: string }
  | {
      action: "insert";
      path: string;
      symbol_id: string;
      position: "before" | "after";
      content: string;
    }
  | { action: "delete"; path: string; symbol_id: string }
  | { action: "comment"; path: string; symbol_id: string; read?: true; content?: string }
  | { action: "edit"; path: string; edits: Array<{ oldText: string; newText: string }> };

function isAction<T extends Action["action"]>(
  input: SrcInput,
  action: T,
): input is Extract<SrcInput, { action: T }> {
  return input.action === action;
}

function isReadComment(
  input: SrcInput,
): input is Extract<SrcInput, { action: "comment"; read: true }> {
  return input.action === "comment" && (input as { read?: boolean }).read === true;
}

const MAX_READ_BYTES = 50 * 1024;

/**
 * Builds the model-facing read text from the CLI's truncation fields using
 * Pi's standard continuation messages.
 */
export function renderReadText(result: ReadResult): string {
  if (result.media) {
    return `Read image file [${result.media.mime}]`;
  }
  const { content, start_line, total_lines, truncated, truncated_by, output_lines, next_offset } =
    result;
  if (result.first_line_exceeds_limit) {
    return `[Line ${start_line} is larger than ${formatSize(MAX_READ_BYTES)}. Use bash to read it in chunks. Full content is available at: ${result.path}]`;
  }
  if (truncated && truncated_by === "lines" && next_offset) {
    const end = start_line + (output_lines ?? 0) - 1;
    return `${content}\n\n[Showing lines ${start_line}-${end} of ${total_lines}. Use offset=${next_offset} to continue. Full content is available at: ${result.path}]`;
  }
  if (truncated && next_offset) {
    const end = start_line + (output_lines ?? 0) - 1;
    return `${content}\n\n[Showing lines ${start_line}-${end} of ${total_lines} (${formatSize(MAX_READ_BYTES)} limit). Use offset=${next_offset} to continue. Full content is available at: ${result.path}]`;
  }
  if (!truncated && output_lines !== undefined && output_lines < total_lines && next_offset) {
    const remaining = total_lines - (start_line + output_lines - 1);
    return `${content}\n\n[${remaining} more lines in file. Use offset=${next_offset} to continue. Full content is available at: ${result.path}]`;
  }
  return content;
}

/** Converts CLI truncation fields to the Pi TruncationResult shape. */
export function toTruncation(result: ReadResult): TruncationResult | undefined {
  if (!result.truncated) {
    return undefined;
  }
  return {
    content: result.content,
    truncated: true,
    truncatedBy: (result.truncated_by ?? "bytes") as "lines" | "bytes" | null,
    totalLines: result.total_lines,
    totalBytes: result.total_bytes,
    outputLines: result.output_lines ?? 0,
    outputBytes: result.output_bytes ?? 0,
    lastLinePartial: false,
    firstLineExceedsLimit: result.first_line_exceeds_limit ?? false,
    maxLines: 2000,
    maxBytes: MAX_READ_BYTES,
  };
}

export function srcTool() {
  const binary = resolveBinaryPath("src", { require });
  return {
    name: "src",
    label: "src",
    description:
      "Structure-aware source file reading and editing: inspect symbol outlines, read files or exact symbols, " +
      "replace/insert/delete/comment symbols by opaque ID, and apply exact multi-edit batches to one file. " +
      "Paths are absolute or relative to the current working directory. Text output is limited to 2,000 lines " +
      "or 50KB; truncated output includes a continuation or full-output location.",
    promptSnippet: "Inspect and edit source files with symbol-aware operations",
    promptGuidelines: SRC_PROMPT_GUIDELINES,
    parameters: srcSchema,
    async execute(
      _toolCallId: string,
      params: SrcInput,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      ctx: { cwd: string; model?: { input?: string[] } },
    ): Promise<{
      content: Array<
        { type: "text"; text: string } | { type: "image"; data: string; mimeType: string }
      >;
      details: unknown;
    }> {
      const absolutePath = resolveSourcePath(params.path, ctx.cwd);
      const run = () => runAndRender(binary, params, absolutePath, signal, ctx.model);
      if (isAction(params, "edit") || isMutation(params)) {
        // Mutations hold Pi's per-file mutation queue for the full
        // child-process read-modify-write window.
        return withFileMutationQueue(absolutePath, run);
      }
      return run();
    },
  };
}

function isMutation(input: SrcInput): boolean {
  return (
    input.action === "replace" ||
    input.action === "insert" ||
    input.action === "delete" ||
    input.action === "comment"
  );
}

async function runAndRender(
  binary: string,
  params: SrcInput,
  absolutePath: string,
  signal: AbortSignal | undefined,
  model?: { input?: string[] },
): Promise<{
  content: Array<
    { type: "text"; text: string } | { type: "image"; data: string; mimeType: string }
  >;
  details: unknown;
}> {
  const { args, stdin } = buildArgs(params, absolutePath);
  const result = await runCli(binary, { args, stdin, signal });
  if (result.exitCode !== 0) {
    throw await cliError(result.stderr, result.exitCode);
  }
  return render(params, result.stdout, model);
}

function buildArgs(input: SrcInput, absolutePath: string): { args: string[]; stdin?: string } {
  if (isAction(input, "symbols")) {
    return { args: ["symbols", absolutePath, "--json"] };
  }
  if (isAction(input, "read")) {
    const args = ["read", absolutePath, "--json"];
    if (input.symbol_id) {
      args.push("--symbol-id", input.symbol_id);
    }
    if (input.offset !== undefined && input.offset > 0) {
      args.push("--offset", String(input.offset));
    }
    if (input.limit !== undefined && input.limit > 0) {
      args.push("--limit", String(input.limit));
    }
    return { args };
  }
  if (isAction(input, "replace")) {
    return {
      args: ["replace", absolutePath, "--symbol-id", input.symbol_id, "--json"],
      stdin: input.content,
    };
  }
  if (isAction(input, "insert")) {
    const flag = input.position === "after" ? "--after" : "--before";
    return {
      args: ["insert", absolutePath, flag, input.symbol_id, "--json"],
      stdin: input.content,
    };
  }
  if (isAction(input, "delete")) {
    return { args: ["delete", absolutePath, "--symbol-id", input.symbol_id, "--json"] };
  }
  if (isAction(input, "comment")) {
    if (isReadComment(input)) {
      return {
        args: ["comment", absolutePath, "--symbol-id", input.symbol_id, "--read", "--json"],
      };
    }
    return {
      args: ["comment", absolutePath, "--symbol-id", input.symbol_id, "--json"],
      stdin: input.content,
    };
  }
  const edits = JSON.stringify({ edits: input.edits });
  return { args: ["edit", absolutePath, "--edits-json", "--json"], stdin: edits };
}

async function render(
  input: SrcInput,
  stdout: string,
  model?: { input?: string[] },
): Promise<{
  content: Array<
    { type: "text"; text: string } | { type: "image"; data: string; mimeType: string }
  >;
  details: unknown;
}> {
  if (isAction(input, "symbols")) {
    const data = parseSingleJsonDoc<SymbolsResult>(stdout);
    const lines = data.symbols.map(
      (s) =>
        `- [${s.id}] ${s.kind} ${s.name}${s.parent ? ` (parent: ${s.parent})` : ""} [L${s.start_line}-L${s.end_line}]${s.has_doc ? " (doc)" : ""}`,
    );
    const text =
      lines.length === 0
        ? "No symbols found."
        : `${data.path} (${data.language}):\n` + lines.join("\n");
    return renderText(data, text, "Use src action symbols again after narrowing the source file.");
  }
  if (isAction(input, "read")) {
    const data = parseSingleJsonDoc<ReadResult>(stdout);
    if (data.media) {
      return renderMedia(data, model);
    }
    const text = renderReadText(data);
    const truncation = toTruncation(data);
    return {
      content: [{ type: "text", text }],
      details: truncation ? { ...data, truncation, fullOutputPath: data.path } : data,
    };
  }
  if (isAction(input, "edit")) {
    const data = parseSingleJsonDoc<EditBatchResult>(stdout);
    return {
      content: [
        {
          type: "text",
          text: `Successfully replaced ${data.edits_applied} block(s) in ${data.path}.`,
        },
      ],
      details: data,
    };
  }
  if (isAction(input, "comment") && isReadComment(input)) {
    const data = parseSingleJsonDoc<CommentReadResult>(stdout);
    return renderText(
      data,
      data.comment,
      "Use src action comment with the same symbol ID to read the comment again.",
    );
  }
  const data = parseSingleJsonDoc<MutationResult>(stdout);
  const label = data.symbol_id ? `${data.action} ${data.symbol_id}` : data.action;
  return {
    content: [{ type: "text", text: `Applied ${label} to ${data.path}.` }],
    details: data,
  };
}

async function renderText(
  data: object,
  raw: string,
  hint: string,
): Promise<{
  content: Array<{ type: "text"; text: string }>;
  details: unknown;
}> {
  const model = await truncateForModel(raw, { hint });
  const details = model.truncation
    ? { ...data, truncation: model.truncation, fullOutputPath: model.fullOutputPath }
    : data;
  return { content: [{ type: "text", text: model.text }], details };
}

const NON_VISION_NOTE =
  "[Current model does not support images. The image will be omitted from this request.]";

function nonVisionNote(model?: { input?: string[] }): string | undefined {
  if (!model || model.input?.includes("image")) {
    return undefined;
  }
  return NON_VISION_NOTE;
}

interface NormalizedInlineImage {
  data: string;
  mimeType: string;
  convertedFrom?: string;
}

function baseImageMimeType(mimeType: string): string {
  return mimeType.split(";")[0]?.trim().toLowerCase() ?? mimeType.toLowerCase();
}

function supportedInlineMimeType(mimeType: string): string | undefined {
  switch (mimeType) {
    case "image/png":
      return "image/png";
    case "image/jpeg":
    case "image/jpg":
      return "image/jpeg";
    case "image/gif":
      return "image/gif";
    case "image/webp":
      return "image/webp";
    default:
      return undefined;
  }
}

// Mirrors Pi's processImage normalization before resizeImage: BMP and any
// other recognized non-inline MIME are decoded to PNG before attachment.
async function normalizeInlineImage(
  data: string,
  mimeType: string,
): Promise<NormalizedInlineImage | undefined> {
  const baseMimeType = baseImageMimeType(mimeType);
  const supportedMimeType = supportedInlineMimeType(baseMimeType);
  if (supportedMimeType) {
    return { data, mimeType: supportedMimeType };
  }
  const converted = await convertToPng(data, mimeType);
  if (!converted) {
    return undefined;
  }
  return { ...converted, convertedFrom: baseMimeType };
}

function mediaOmittedText(mimeType: string, message: string, note?: string): string {
  return [`Read image file [${mimeType}]`, message, note].filter(Boolean).join("\n");
}

async function renderMedia(
  data: ReadResult,
  model?: { input?: string[] },
): Promise<{
  content: Array<
    { type: "text"; text: string } | { type: "image"; data: string; mimeType: string }
  >;
  details: unknown;
}> {
  const media = data.media!;
  const note = nonVisionNote(model);
  const normalized = await normalizeInlineImage(media.data_base64, media.mime);
  if (!normalized) {
    return {
      content: [
        {
          type: "text",
          text: mediaOmittedText(
            media.mime,
            "[Image omitted: could not be converted to a supported inline image format.]",
            note,
          ),
        },
      ],
      details: { mime: media.mime },
    };
  }

  const resized = await resizeImage(Buffer.from(normalized.data, "base64"), normalized.mimeType, {
    maxWidth: 2000,
    maxHeight: 2000,
  });
  if (!resized) {
    return {
      content: [
        {
          type: "text",
          text: mediaOmittedText(
            normalized.mimeType,
            "[Image omitted: could not be resized below the inline image size limit.]",
            note,
          ),
        },
      ],
      details: { mime: media.mime },
    };
  }

  const hints = [
    normalized.convertedFrom && normalized.convertedFrom !== resized.mimeType
      ? `[Image converted from ${normalized.convertedFrom} to ${resized.mimeType}.]`
      : undefined,
    formatDimensionNote(resized),
    note,
  ].filter((hint): hint is string => Boolean(hint));
  return {
    content: [
      {
        type: "text",
        text: [`Read image file [${resized.mimeType}]`, ...hints].join("\n"),
      },
      { type: "image", data: resized.data, mimeType: resized.mimeType },
    ],
    details: { mime: media.mime, width: resized.width, height: resized.height },
  };
}

export function registerSrcTool(pi: {
  registerTool(definition: ReturnType<typeof srcTool>): void;
}): void {
  pi.registerTool(srcTool());
}
