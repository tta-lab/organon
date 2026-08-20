import {
  credentialRef,
  type CredentialRef,
  type ResolvedCredential,
} from "@deepseek-ai/dsh-credentials";
import type { Context } from "@deepseek-ai/cordis";
import { defineTool, type ToolDefinition } from "@deepseek-ai/dsh-tools";

import { fetchWebPage, type FetchInput, type FetchResult } from "@tta-lab/pi-shared/fetch";
import { parseSingleJsonDoc } from "@tta-lab/pi-shared/parse";
import { runCli, type CliRunOptions, type CliRunResult } from "@tta-lab/pi-shared/process";
import { formatSize, truncateHead } from "@tta-lab/pi-shared/truncate-core";

import { CONTEXT7_CREDENTIAL_REF } from "./contract.js";

const DEFAULT_TREE_THRESHOLD = 5000;

export interface WebToolDependencies {
  binaryPath: string;
  credentials: {
    resolve(ref: CredentialRef): Promise<ResolvedCredential | undefined>;
  };
  fetch?: (input: FetchInput, signal?: AbortSignal) => Promise<FetchResult>;
  run?: (binaryPath: string, options: CliRunOptions) => Promise<CliRunResult>;
}

interface DocsLibrary {
  id: string;
  title: string;
  description?: string;
  trust_score?: number;
  total_snippets?: number;
  versions?: string[];
}

interface DocsResolveResult {
  query: string;
  libraries: DocsLibrary[];
}

interface DocsFetchResult {
  library_id: string;
  topic?: string;
  content: string;
}

interface SGraphResult {
  content: string;
}

type DocsInput =
  | { action: "resolve"; query: string }
  | { action: "fetch"; library_id: string; topic?: string; tokens?: number };

const fetchParameters = {
  url: { type: "string", required: true, description: "HTTP or HTTPS URL to fetch" },
  tree: { type: "boolean", description: "Show the page heading tree" },
  section_id: { type: "string", description: "Optional heading section ID to return" },
  full: { type: "boolean", description: "Return full content without automatic tree mode" },
  tree_threshold: {
    type: "integer",
    default: DEFAULT_TREE_THRESHOLD,
    description: "Automatic tree threshold; defaults to 5000",
  },
} as const;

const docsParameters = {
  action: {
    type: "string",
    enum: ["resolve", "fetch"],
    required: true,
    description: "Documentation operation",
  },
  query: { type: "string", description: "Library name or package query" },
  library_id: { type: "string", description: "Context7 library ID returned by resolve" },
  topic: { type: "string", description: "Optional documentation topic" },
  tokens: {
    type: "integer",
    default: 0,
    description: "Optional token budget for fetch; zero uses the backend default",
  },
} as const;

const sgraphParameters = {
  query: { type: "string", required: true, description: "Sourcegraph search query" },
  count: { type: "integer", default: 10, description: "Optional result count; defaults to 10" },
  context: { type: "integer", default: 10, description: "Optional context lines; defaults to 10" },
  timeout: {
    type: "integer",
    default: 0,
    description: "Optional timeout in seconds; zero disables the timeout",
  },
} as const;

const fetchOutput = {
  schema: {
    type: "object",
    additionalProperties: false,
    properties: {
      url: { type: "string", required: true },
      mode: { type: "string", enum: ["full", "tree", "section"], required: true },
      content: { type: "string", required: true },
    },
  } as const,
  render: (_args: unknown, value: FetchResult) => [
    {
      type: "text" as const,
      text: boundedToolText(
        value.content,
        "Use web_fetch with tree or section_id to navigate the document.",
      ),
    },
  ],
};

const docsOutput = {
  schema: {
    oneOf: [
      {
        type: "object",
        additionalProperties: false,
        properties: {
          query: { type: "string", required: true },
          libraries: {
            type: "array",
            required: true,
            items: {
              type: "object",
              additionalProperties: false,
              properties: {
                id: { type: "string", required: true },
                title: { type: "string", required: true },
                description: { type: "string" },
                trust_score: { type: "number" },
                total_snippets: { type: "integer" },
                versions: { type: "array", items: { type: "string" } },
              },
            },
          },
        },
      },
      {
        type: "object",
        additionalProperties: false,
        properties: {
          library_id: { type: "string", required: true },
          topic: { type: "string" },
          content: { type: "string", required: true },
        },
      },
    ],
  } as const,
  render: (_args: unknown, value: DocsResolveResult | DocsFetchResult) => [
    {
      type: "text" as const,
      text:
        "libraries" in value
          ? boundedToolText(
              formatDocsResolve(value),
              "Use web_docs action fetch with a library_id from the results.",
            )
          : boundedToolText(
              value.content,
              "Refetch with a narrower topic or tokens budget for the remainder.",
            ),
    },
  ],
};

const sgraphOutput = {
  schema: {
    type: "object",
    additionalProperties: false,
    properties: { content: { type: "string", required: true } },
  } as const,
  render: (_args: unknown, value: SGraphResult) => [
    {
      type: "text" as const,
      text: boundedToolText(
        value.content,
        "Use a narrower Sourcegraph query or lower count and context.",
      ),
    },
  ],
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function rejectUnknownFields(input: unknown, fields: readonly string[], toolName: string): void {
  if (!isRecord(input)) return;
  for (const field of Object.keys(input)) {
    if (!fields.includes(field)) {
      throw new Error(`${toolName} input does not accept field ${field}`);
    }
  }
}

function strictDefinition(definition: ToolDefinition): ToolDefinition {
  return {
    ...definition,
    parameters: { ...definition.parameters, additionalProperties: false },
  };
}

function requireString(input: unknown, field: string): string {
  if (!isRecord(input) || typeof input[field] !== "string" || input[field].length === 0) {
    throw new Error(`${field} must be a non-empty string`);
  }
  return input[field] as string;
}

function normalizeDocs(input: unknown): DocsInput {
  if (!isRecord(input)) {
    throw new Error("web_docs input must be an object");
  }
  for (const field of Object.keys(input)) {
    if (!["action", "query", "library_id", "topic", "tokens"].includes(field)) {
      throw new Error(`web_docs input does not accept field ${field}`);
    }
  }
  if (input.action !== "resolve" && input.action !== "fetch") {
    throw new Error('web_docs action must be "resolve" or "fetch"');
  }
  if (input.action === "resolve") {
    if ("library_id" in input || "topic" in input || "tokens" in input) {
      throw new Error('web_docs action "resolve" does not accept library_id, topic, or tokens');
    }
    return { action: "resolve", query: requireString(input, "query") };
  }
  if ("query" in input) {
    throw new Error('web_docs action "fetch" does not accept query');
  }
  const library_id = requireString(input, "library_id");
  if (input.topic !== undefined && typeof input.topic !== "string") {
    throw new Error("topic must be a string");
  }
  if (
    input.tokens !== undefined &&
    (!Number.isInteger(input.tokens) || typeof input.tokens !== "number")
  ) {
    throw new Error("tokens must be an integer");
  }
  return {
    action: "fetch",
    library_id,
    topic: input.topic as string | undefined,
    tokens: input.tokens as number | undefined,
  };
}

async function context7Credential(dependencies: WebToolDependencies): Promise<{
  env?: NodeJS.ProcessEnv;
}> {
  let resolved: ResolvedCredential | undefined;
  try {
    resolved = await dependencies.credentials.resolve(credentialRef(CONTEXT7_CREDENTIAL_REF));
  } catch {
    throw new Error("Context7 credential resolution failed");
  }
  if (resolved === undefined) return {};
  return { env: { CONTEXT7_API_KEY: resolved.value } };
}

function operationName(args: readonly string[]): string {
  if (args[0] === "docs") return `web docs ${args[1] ?? "operation"}`;
  return `web ${args[0] ?? "child"}`;
}

function boundedToolText(content: string, hint: string): string {
  const truncation = truncateHead(content);
  if (!truncation.truncated) return content;
  const guidance = hint.trim() === "" ? "" : ` ${hint.trim()}`;
  if (truncation.firstLineExceedsLimit) {
    return `[First line is ${formatSize(truncation.totalBytes)}, exceeds ${formatSize(truncation.maxBytes)} limit.${guidance}]`;
  }
  if (truncation.truncatedBy === "lines") {
    return `${truncation.content}\n\n[Truncated: showing ${truncation.outputLines} of ${truncation.totalLines} lines (${truncation.maxLines} line limit).${guidance}]`;
  }
  return `${truncation.content}\n\n[Truncated: ${truncation.outputLines} lines shown (${formatSize(truncation.maxBytes)} limit).${guidance}]`;
}

function childError(operation: string, exitCode: number): Error {
  return new Error(`${operation} failed: command exited with code ${exitCode}`);
}

async function runJSON<T>(
  dependencies: WebToolDependencies,
  args: string[],
  signal: AbortSignal,
  env?: NodeJS.ProcessEnv,
): Promise<T> {
  const operation = operationName(args);
  const runner = dependencies.run ?? runCli;
  let result: CliRunResult;
  try {
    result = await runner(dependencies.binaryPath, { args, signal, ...(env ? { env } : {}) });
  } catch (error) {
    if (error instanceof Error && error.message === "Operation aborted") throw error;
    throw new Error(`${operation} failed to start`);
  }
  if (result.exitCode !== 0) {
    throw childError(operation, result.exitCode);
  }
  try {
    return parseSingleJsonDoc<T>(result.stdout);
  } catch {
    throw new Error(`${operation} returned invalid JSON`);
  }
}

function formatDocsResolve(result: DocsResolveResult): string {
  if (result.libraries.length === 0)
    return `No libraries found for ${JSON.stringify(result.query)}`;
  return `Found ${result.libraries.length} libraries:\n${result.libraries
    .map((library) => `- ${library.id}: ${library.title}`)
    .join("\n")}`;
}

function webFetchTool(dependencies: WebToolDependencies): ToolDefinition {
  const fetch = dependencies.fetch ?? fetchWebPage;
  return strictDefinition(
    defineTool({
      name: "web_fetch",
      description:
        "Fetch and read an HTTP or HTTPS web page as Markdown with heading-tree and section navigation.",
      parameters: fetchParameters,
      output: fetchOutput,
      isConcurrencySafe: () => true,
      async execute(args, exec) {
        rejectUnknownFields(args, Object.keys(fetchParameters), "web_fetch");
        return fetch(
          {
            url: requireString(args, "url"),
            tree: args.tree,
            section_id: args.section_id,
            full: args.full,
            tree_threshold: args.tree_threshold,
          },
          exec.signal,
        );
      },
    }),
  );
}

function webDocsTool(dependencies: WebToolDependencies): ToolDefinition {
  return strictDefinition(
    defineTool({
      name: "web_docs",
      description:
        "Resolve a library and fetch its documentation through Context7. Use action resolve with query, then action fetch with library_id.",
      parameters: docsParameters,
      output: docsOutput,
      isConcurrencySafe: () => true,
      async execute(args, exec) {
        const input = normalizeDocs(args);
        const credential = await context7Credential(dependencies);
        const argsForChild =
          input.action === "resolve"
            ? ["docs", "resolve", "--json", "--", input.query]
            : [
                "docs",
                "fetch",
                ...(input.tokens !== undefined && input.tokens !== 0
                  ? ["--tokens", String(input.tokens)]
                  : []),
                "--json",
                "--",
                input.library_id,
                ...(input.topic !== undefined && input.topic !== "" ? [input.topic] : []),
              ];
        if (input.action === "resolve") {
          return runJSON<DocsResolveResult>(
            dependencies,
            argsForChild,
            exec.signal,
            credential.env,
          );
        }
        return runJSON<DocsFetchResult>(dependencies, argsForChild, exec.signal, credential.env);
      },
    }),
  );
}

function webSgraphTool(dependencies: WebToolDependencies): ToolDefinition {
  return strictDefinition(
    defineTool({
      name: "web_sgraph",
      description: "Search public source code through Sourcegraph.",
      parameters: sgraphParameters,
      output: sgraphOutput,
      isConcurrencySafe: () => true,
      async execute(args, exec) {
        rejectUnknownFields(args, Object.keys(sgraphParameters), "web_sgraph");
        const query = requireString(args, "query");
        const argsForChild = ["sgraph", "--json"];
        if (args.count !== undefined && args.count !== 10) {
          argsForChild.push("--count", String(args.count));
        }
        if (args.context !== undefined && args.context !== 10) {
          argsForChild.push("--context", String(args.context));
        }
        if (args.timeout !== undefined && args.timeout !== 0) {
          argsForChild.push("--timeout", String(args.timeout));
        }
        argsForChild.push("--", query);
        return runJSON<SGraphResult>(dependencies, argsForChild, exec.signal);
      },
    }),
  );
}

export function createWebToolDefinitions(
  dependencies: WebToolDependencies,
): readonly [ToolDefinition, ToolDefinition, ToolDefinition] {
  return [webFetchTool(dependencies), webDocsTool(dependencies), webSgraphTool(dependencies)];
}

export function registerWebTools(ctx: Context, dependencies: WebToolDependencies): void {
  for (const definition of createWebToolDefinitions(dependencies)) {
    ctx.tools.register(definition);
  }
}
