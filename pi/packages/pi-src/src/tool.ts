import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  convertToPng,
  createEditToolDefinition,
  createReadToolDefinition,
  formatDimensionNote,
  formatSize,
  resizeImage,
  type EditToolDetails,
  type ReadToolDetails,
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

// These are deliberately created from the installed Pi factories. The peer
// dependency is the source of truth for the built-in schema and prompt
// contribution, so the override follows the Pi version that hosts it.
const builtInRead = createReadToolDefinition(process.cwd());
const builtInEdit = createEditToolDefinition(process.cwd());

// Pi's current TypeBox object schemas intentionally omit an
// additionalProperties keyword. Close only the top-level built-in branches in
// the override copy so Organon fields cannot accidentally mix into an
// otherwise valid exact call; the live schema and its nested edit entries are
// still sourced from the installed factory unchanged.
const builtInReadParameters = { ...builtInRead.parameters, additionalProperties: false };
const builtInEditParameters = { ...builtInEdit.parameters, additionalProperties: false };

const pathDescription = "Path to the file (absolute, or relative to the current working directory)";
const symbolIdDescription =
  "Exact opaque symbol or Markdown section ID returned by read with symbols: true; use symbol_id, not symbol or a display name.";
const symbolId = Type.String({ description: symbolIdDescription, minLength: 1 });

const symbolsReadSchema = Type.Object(
  {
    path: Type.String({ description: pathDescription }),
    symbols: Type.Boolean({
      description: "Return the file's current symbol or Markdown section outline",
      enum: [true],
    }),
  },
  { additionalProperties: false },
);

const symbolReadSchema = Type.Object(
  {
    path: Type.String({ description: pathDescription }),
    symbol_id: symbolId,
    offset: Type.Optional(
      Type.Integer({
        description: "1-indexed line offset within the selected symbol or section",
        minimum: 1,
      }),
    ),
    limit: Type.Optional(
      Type.Integer({
        description: "Maximum number of lines in the selected symbol or section",
        minimum: 1,
      }),
    ),
  },
  { additionalProperties: false },
);

/** The read override is an explicit union with the live built-in branch. */
export const readSchema = Type.Union([builtInReadParameters, symbolsReadSchema, symbolReadSchema]);

const replaceSymbolSchema = Type.Object(
  {
    path: Type.String({ description: pathDescription }),
    operation: StringEnum(["replace"] as const, {
      description: "Replace one exact symbol or section",
    }),
    symbol_id: symbolId,
    content: Type.String({ description: "Replacement content (may be multiline)" }),
  },
  { additionalProperties: false },
);

const insertSymbolSchema = Type.Object(
  {
    path: Type.String({ description: pathDescription }),
    operation: StringEnum(["insert"] as const, {
      description: "Insert around one exact symbol or section",
    }),
    symbol_id: symbolId,
    position: StringEnum(["before", "after"] as const, {
      description: "Insert before or after the symbol or section",
    }),
    content: Type.String({ description: "Content to insert (may be multiline)" }),
  },
  { additionalProperties: false },
);

const deleteSymbolSchema = Type.Object(
  {
    path: Type.String({ description: pathDescription }),
    operation: StringEnum(["delete"] as const, {
      description: "Delete one exact symbol or section",
    }),
    symbol_id: symbolId,
  },
  { additionalProperties: false },
);

const commentSymbolSchema = Type.Object(
  {
    path: Type.String({ description: pathDescription }),
    operation: StringEnum(["comment"] as const, {
      description: "Replace one exact symbol doc comment",
    }),
    symbol_id: symbolId,
    content: Type.String({ description: "New doc comment content (may be multiline)" }),
  },
  { additionalProperties: false },
);

/** The edit override is an explicit union with the live built-in branch. */
export const editSchema = Type.Union([
  builtInEditParameters,
  replaceSymbolSchema,
  insertSymbolSchema,
  deleteSymbolSchema,
  commentSymbolSchema,
]);

export type ReadInput = Static<typeof readSchema>;
export type EditInput = Static<typeof editSchema>;

type TextContentBlock = { type: "text"; text: string };
type ImageContentBlock = { type: "image"; data: string; mimeType: string };
type ReadContentBlock = TextContentBlock | ImageContentBlock;

interface SymbolsResult {
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

interface ReadResult {
  path: string;
  symbol_id?: string;
  content: string;
  start_line: number;
  total_lines: number;
  truncation_total_lines: number;
  total_bytes: number;
  truncated: boolean;
  truncated_by?: string;
  output_lines?: number;
  output_bytes?: number;
  output_end_line?: number;
  remaining_lines?: number;
  next_offset?: number;
  first_line_exceeds_limit?: boolean;
  media?: { kind: string; mime: string; data_base64: string };
}

interface MutationResult {
  path: string;
  action: string;
  symbol_id?: string;
  diff: string;
  patch: string;
  first_changed_line?: number;
  outline: SymbolsResult;
}

interface EditBatchResult {
  path: string;
  diff: string;
  patch: string;
  first_changed_line?: number;
  edits_applied: number;
}

const READ_PROMPT_SNIPPET =
  "For source symbols and Markdown heading sections, use read({ path, symbols: true }) first, then read({ path, symbol_id: id })";
const EDIT_PROMPT_SNIPPET =
  "Use edit({ path, operation: 'replace', symbol_id: id, content }) for source symbols or Markdown sections, or edit({ path, edits: [{ oldText, newText }] }) for exact text";

const SYMBOL_ID_STABILITY_GUIDANCE =
  "Opaque IDs are deterministic from canonical symbol or heading labels: body, content, and line-only edits normally preserve unchanged IDs, while renames or structural changes may not; treat the latest returned outline as authoritative.";

const READ_PROMPT_GUIDELINES = [
  "Prefer read's symbol-aware navigation for structured source and Markdown when the exact text is not already known.",
  "Source symbols and every Markdown heading section, including H1, share the same outline → opaque symbol_id workflow: get the outline first, then copy its exact returned ID; never use symbol or a display name.",
  "A symbol-scoped read uses the returned symbol_id for either a source symbol or a Markdown heading section.",
  SYMBOL_ID_STABILITY_GUIDANCE,
  "Continue from a post-edit outline returned by edit; request read with symbols: true again only when another edit may have made IDs stale or the needed entry was omitted.",
];

const EDIT_PROMPT_GUIDELINES = [
  "Prefer edit's symbol-aware operations for structured source and Markdown when the exact original text is not already known.",
  "Source symbols and every Markdown heading section, including H1, share the same outline → opaque symbol_id workflow for symbol-scoped read, replace, insert, and delete; source comment operations use that same opaque symbol_id form for documentation.",
  "Before the first symbol-scoped edit, get the outline with read's symbols form and copy its exact opaque ID into symbol_id; never use symbol or a display name.",
  SYMBOL_ID_STABILITY_GUIDANCE,
  "After a symbol mutation, continue from the post-edit outline returned in that edit result instead of making a redundant outline read; refresh only when a later edit may have made IDs stale or truncation omitted the needed entry.",
  "Keep symbol replacement and exact-text replacement distinct: the former uses operation, symbol_id, and content; the latter uses edits[] with oldText and newText.",
  "Use normal exact edit directly when the original text is already known; a later exact edit may make previously returned symbol IDs stale.",
  "For multiple disjoint exact replacements, use one edit call with multiple entries in edits[].",
];

const MAX_READ_BYTES = 50 * 1024;

interface ReadExecutionResult {
  content: ReadContentBlock[];
  details: ReadToolDetails | undefined;
}

interface EditExecutionResult {
  content: TextContentBlock[];
  details: EditToolDetails | undefined;
}

interface SymbolReadInput {
  path: string;
  symbol_id: string;
  offset?: number;
  limit?: number;
}

interface SymbolsReadInput {
  path: string;
  symbols: true;
}

type SymbolEditInput =
  | { path: string; operation: "replace"; symbol_id: string; content: string }
  | {
      path: string;
      operation: "insert";
      symbol_id: string;
      position: "before" | "after";
      content: string;
    }
  | { path: string; operation: "delete"; symbol_id: string }
  | { path: string; operation: "comment"; symbol_id: string; content: string };

function isRecord(input: unknown): input is Record<string, unknown> {
  return typeof input === "object" && input !== null;
}

function isSymbolsRead(input: ReadInput): input is SymbolsReadInput {
  const value = input as unknown as Record<string, unknown>;
  return isRecord(value) && value["symbols"] === true && typeof value["path"] === "string";
}

function isSymbolRead(input: ReadInput): input is SymbolReadInput {
  const value = input as unknown as Record<string, unknown>;
  return isRecord(value) && typeof value["symbol_id"] === "string";
}

function isSymbolEdit(input: EditInput): input is SymbolEditInput {
  const value = input as unknown as Record<string, unknown>;
  return isRecord(value) && typeof value["operation"] === "string";
}

function hasEmptySymbolID(input: unknown): boolean {
  return isRecord(input) && typeof input.symbol_id === "string" && input.symbol_id.length === 0;
}

function assertSymbolID(input: unknown): void {
  if (hasEmptySymbolID(input)) {
    throw new Error("symbol_id must not be empty");
  }
}

function appendReadWindow(args: string[], offset?: number, limit?: number): void {
  // Pi treats zero and negative offsets as the start of the file, so omitting
  // those values is behaviorally equivalent and keeps the CLI's one-indexed
  // offset validation intact. A present limit is different: limit=0 selects
  // no lines and must not be treated as an omitted limit.
  if (offset !== undefined && offset > 0) {
    const value = Math.trunc(offset);
    if (value > 0) {
      args.push("--offset", String(value));
    }
  }
  if (limit !== undefined) {
    args.push("--limit", String(Math.trunc(limit)));
  }
}

function buildReadArgs(input: ReadInput, absolutePath: string): { args: string[] } {
  assertSymbolID(input);
  if (isSymbolsRead(input)) {
    return { args: ["symbols", absolutePath, "--json"] };
  }
  if (isSymbolRead(input)) {
    const args = ["read", absolutePath, "--symbol-id", input.symbol_id, "--json"];
    appendReadWindow(args, input.offset, input.limit);
    return { args };
  }

  const exactInput = input as { offset?: number; limit?: number };
  const args = ["read", absolutePath, "--json"];
  appendReadWindow(args, exactInput.offset, exactInput.limit);
  return { args };
}

function buildEditArgs(input: EditInput, absolutePath: string): { args: string[]; stdin?: string } {
  assertSymbolID(input);
  if (isSymbolEdit(input)) {
    switch (input.operation) {
      case "replace":
        return {
          args: ["replace", absolutePath, "--symbol-id", input.symbol_id, "--json"],
          stdin: input.content,
        };
      case "insert": {
        const flag = input.position === "after" ? "--after" : "--before";
        return {
          args: ["insert", absolutePath, flag, input.symbol_id, "--json"],
          stdin: input.content,
        };
      }
      case "delete":
        return { args: ["delete", absolutePath, "--symbol-id", input.symbol_id, "--json"] };
      case "comment":
        return {
          args: ["comment", absolutePath, "--symbol-id", input.symbol_id, "--json"],
          stdin: input.content,
        };
      default:
        throw new Error("unsupported edit operation");
    }
  }

  if (!isRecord(input) || !Array.isArray(input.edits)) {
    throw new Error("edit input must contain edits or one symbol operation");
  }
  return {
    args: ["edit", absolutePath, "--edits-json", "--json"],
    stdin: JSON.stringify({ edits: input.edits }),
  };
}

function renderReadText(result: ReadResult): string {
  if (result.media) {
    return `Read image file [${result.media.mime}]`;
  }
  const {
    content,
    start_line,
    total_lines,
    truncated,
    truncated_by,
    output_end_line,
    remaining_lines,
    next_offset,
  } = result;
  if (result.first_line_exceeds_limit) {
    return `[Line ${start_line} is larger than ${formatSize(MAX_READ_BYTES)}. Use bash to read it in chunks. Full content is available at: ${result.path}]`;
  }
  if (truncated && truncated_by === "lines" && next_offset) {
    return `${content}\n\n[Showing lines ${start_line}-${output_end_line} of ${total_lines}. Use offset=${next_offset} to continue. Full content is available at: ${result.path}]`;
  }
  if (truncated && next_offset) {
    return `${content}\n\n[Showing lines ${start_line}-${output_end_line} of ${total_lines} (${formatSize(MAX_READ_BYTES)} limit). Use offset=${next_offset} to continue. Full content is available at: ${result.path}]`;
  }
  if (!truncated && next_offset) {
    return `${content}\n\n[${remaining_lines} more lines in file. Use offset=${next_offset} to continue. Full content is available at: ${result.path}]`;
  }
  return content;
}

/** Converts the CLI's Pi-equivalent window into built-in read details. */
function toTruncation(result: ReadResult): TruncationResult | undefined {
  if (!result.truncated) {
    return undefined;
  }
  return {
    content: result.content,
    truncated: true,
    truncatedBy: (result.truncated_by ?? "bytes") as "lines" | "bytes" | null,
    totalLines: result.truncation_total_lines,
    totalBytes: result.total_bytes,
    outputLines: result.output_lines ?? 0,
    outputBytes: result.output_bytes ?? 0,
    lastLinePartial: false,
    firstLineExceedsLimit: result.first_line_exceeds_limit ?? false,
    maxLines: 2000,
    maxBytes: MAX_READ_BYTES,
  };
}

function readTextResult(text: string, truncation?: TruncationResult): ReadExecutionResult {
  return {
    content: [{ type: "text", text }],
    details: truncation ? { truncation } : undefined,
  };
}

const OUTLINE_TRUNCATION_HINT =
  "Continue from this outline; use read with symbols: true again only when another edit may have made IDs stale or a needed entry was omitted.";

function renderOutlineText(data: SymbolsResult): string {
  const lines = data.symbols.map(
    (symbol) =>
      `- [${symbol.id}] ${symbol.kind} ${symbol.name}${symbol.parent ? ` (parent: ${symbol.parent})` : ""} [L${symbol.start_line}-L${symbol.end_line}]${symbol.has_doc ? " (doc)" : ""}`,
  );
  return lines.length === 0
    ? "No symbols found."
    : `${data.path} (${data.language}):\n` + lines.join("\n");
}

async function renderOutline(data: SymbolsResult) {
  return truncateForModel(renderOutlineText(data), { hint: OUTLINE_TRUNCATION_HINT });
}

async function renderSymbols(data: SymbolsResult): Promise<ReadExecutionResult> {
  const model = await renderOutline(data);
  return readTextResult(model.text, model.truncation);
}

function readResult(data: ReadResult, model?: { input?: string[] }): Promise<ReadExecutionResult> {
  if (data.media) {
    return renderMedia(data, model);
  }
  return Promise.resolve(readTextResult(renderReadText(data), toTruncation(data)));
}

function editDetails(
  data: Pick<MutationResult, "diff" | "patch" | "first_changed_line">,
): EditToolDetails {
  const details: EditToolDetails = { diff: data.diff, patch: data.patch };
  if (data.first_changed_line !== undefined) {
    details.firstChangedLine = data.first_changed_line;
  }
  return details;
}

async function renderEdit(input: EditInput, stdout: string): Promise<EditExecutionResult> {
  if (!isSymbolEdit(input)) {
    const data = parseSingleJsonDoc<EditBatchResult>(stdout);
    return {
      content: [
        {
          type: "text",
          text: `Successfully replaced ${data.edits_applied} block(s) in ${data.path}.`,
        },
      ],
      details: editDetails(data),
    };
  }

  const data = parseSingleJsonDoc<MutationResult>(stdout);
  const label = data.symbol_id ? `${data.action} ${data.symbol_id}` : data.action;
  const outline = await renderOutline(data.outline);
  return {
    // Truncate only the outline portion so the success confirmation is always
    // visible; diff and patch remain in the built-in-compatible details.
    content: [
      {
        type: "text",
        text: `Applied ${label} to ${data.path}.\n\nPost-edit outline:\n${outline.text}`,
      },
    ],
    details: editDetails(data),
  };
}

async function runRead(
  binary: string,
  input: ReadInput,
  absolutePath: string,
  signal: AbortSignal | undefined,
  model?: { input?: string[] },
): Promise<ReadExecutionResult> {
  const { args } = buildReadArgs(input, absolutePath);
  const result = await runCli(binary, { args, signal });
  if (result.exitCode !== 0) {
    throw await cliError(result.stderr, result.exitCode);
  }
  const data = parseSingleJsonDoc<SymbolsResult | ReadResult>(result.stdout);
  if (isSymbolsRead(input)) {
    return renderSymbols(data as SymbolsResult);
  }
  return readResult(data as ReadResult, model);
}

async function runEdit(
  binary: string,
  input: EditInput,
  absolutePath: string,
  signal: AbortSignal | undefined,
): Promise<EditExecutionResult> {
  const { args, stdin } = buildEditArgs(input, absolutePath);
  const result = await runCli(binary, { args, stdin, signal });
  if (result.exitCode !== 0) {
    throw await cliError(result.stderr, result.exitCode);
  }
  return renderEdit(input, result.stdout);
}

export function readTool() {
  const binary = resolveBinaryPath("src", { require });
  return {
    name: "read",
    label: builtInRead.label,
    description:
      `${builtInRead.description} ` +
      "It also supports an outline-first symbol workflow: use symbols: true, then the returned opaque ID as symbol_id (not symbol) to read one exact symbol or Markdown section.",
    promptSnippet: [builtInRead.promptSnippet, READ_PROMPT_SNIPPET].filter(Boolean).join("; "),
    promptGuidelines: [...(builtInRead.promptGuidelines ?? []), ...READ_PROMPT_GUIDELINES],
    parameters: readSchema,
    async execute(
      _toolCallId: string,
      params: ReadInput,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      ctx: { cwd: string; model?: { input?: string[] } },
    ): Promise<ReadExecutionResult> {
      const absolutePath = resolveSourcePath(params.path, ctx.cwd);
      return runRead(binary, params, absolutePath, signal, ctx.model);
    },
  };
}

function isOrganonEditArguments(input: unknown): boolean {
  return isRecord(input) && "operation" in input;
}

export function editTool() {
  const binary = resolveBinaryPath("src", { require });
  return {
    name: "edit",
    label: builtInEdit.label,
    description:
      `${builtInEdit.description} ` +
      "It also supports replace, insert, delete, and comment operations against exact opaque symbol or Markdown section IDs. Symbol replacement uses operation, symbol_id, and content; exact text uses edits[] entries with oldText and newText.",
    promptSnippet: [builtInEdit.promptSnippet, EDIT_PROMPT_SNIPPET].filter(Boolean).join("; "),
    promptGuidelines: [...(builtInEdit.promptGuidelines ?? []), ...EDIT_PROMPT_GUIDELINES],
    parameters: editSchema,
    // Let Pi's installed edit compatibility shim normalize familiar exact forms
    // (including host-side positional/alias preparation) before this union is
    // validated. Organon operation objects are passed through unchanged.
    prepareArguments(args: unknown): Static<typeof editSchema> {
      if (isOrganonEditArguments(args)) {
        return args as Static<typeof editSchema>;
      }
      return (builtInEdit.prepareArguments?.(args) ?? args) as Static<typeof editSchema>;
    },
    async execute(
      _toolCallId: string,
      params: EditInput,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      ctx: { cwd: string },
    ): Promise<EditExecutionResult> {
      const absolutePath = resolveSourcePath(params.path, ctx.cwd);
      const run = () => runEdit(binary, params, absolutePath, signal);
      return withFileMutationQueue(absolutePath, run);
    },
  };
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
): Promise<ReadExecutionResult> {
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
      details: undefined,
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
      details: undefined,
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
    details: undefined,
  };
}

export function registerReadEditTools(pi: Pick<ExtensionAPI, "registerTool">): void {
  pi.registerTool(readTool());
  pi.registerTool(editTool());
}
