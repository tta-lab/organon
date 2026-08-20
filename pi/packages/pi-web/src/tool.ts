import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import { Type, type Static } from "typebox";

import {
  cliError,
  detectPlatform,
  modelTextResult,
  parseSingleJsonDoc,
  resolveBinaryPath,
  runCli,
} from "@tta-lab/pi-shared";

import { fetchWebPage, type FetchResult } from "@tta-lab/pi-shared/fetch";
export type { FetchResult } from "@tta-lab/pi-shared/fetch";

const require = createRequire(import.meta.url);
const DEFAULT_TREE_THRESHOLD = 5000;

export const webSearchSchema = Type.Object(
  {
    queries: Type.Array(
      Type.String({ description: "Web search query", minLength: 1, pattern: "\\S" }),
      {
        description: "One to four web search queries",
        minItems: 1,
        maxItems: 4,
      },
    ),
  },
  { additionalProperties: false },
);

export const webFetchSchema = Type.Object(
  {
    url: Type.String({ description: "HTTP or HTTPS URL to fetch" }),
    tree: Type.Optional(Type.Boolean({ description: "Show the page heading tree" })),
    section_id: Type.Optional(
      Type.String({ description: "Optional heading section ID to return" }),
    ),
    full: Type.Optional(
      Type.Boolean({ description: "Return full content without automatic tree mode" }),
    ),
    tree_threshold: Type.Optional(
      Type.Integer({
        description: "Automatic tree threshold; defaults to " + DEFAULT_TREE_THRESHOLD,
        default: DEFAULT_TREE_THRESHOLD,
      }),
    ),
  },
  { additionalProperties: false },
);

export const webDocsSchema = Type.Object(
  {
    action: StringEnum(["resolve", "fetch"] as const, {
      description: "Documentation operation",
    }),
    query: Type.Optional(Type.String({ description: "Library name or package query" })),
    library_id: Type.Optional(
      Type.String({ description: "Context7 library ID returned by resolve" }),
    ),
    topic: Type.Optional(Type.String({ description: "Optional documentation topic" })),
    tokens: Type.Optional(
      Type.Integer({
        description: "Optional token budget for fetch; zero uses the backend default",
        default: 0,
      }),
    ),
  },
  { additionalProperties: false },
);

export const webSgraphSchema = Type.Object(
  {
    query: Type.String({ description: "Sourcegraph search query" }),
    count: Type.Optional(
      Type.Integer({ description: "Optional result count; defaults to 10", default: 10 }),
    ),
    context: Type.Optional(
      Type.Integer({ description: "Optional context lines; defaults to 10", default: 10 }),
    ),
    timeout: Type.Optional(
      Type.Integer({
        description: "Optional timeout in seconds; zero disables the timeout",
        default: 0,
      }),
    ),
  },
  { additionalProperties: false },
);

export type WebSearchInput = Static<typeof webSearchSchema>;
export type WebFetchInput = Static<typeof webFetchSchema>;
export type WebDocsInput = Static<typeof webDocsSchema>;
export type WebSgraphInput = Static<typeof webSgraphSchema>;

type DocsInput =
  | { action: "resolve"; query: string }
  | { action: "fetch"; library_id: string; topic?: string; tokens?: number };

export interface SearchResult {
  provider: string;
  results: Array<{ title: string; link: string; snippet: string; position: number }>;
}

export interface DocsLibrary {
  id: string;
  title: string;
  trust_score?: number;
  total_snippets?: number;
  versions?: string[];
  description?: string;
}

export interface DocsResolveResult {
  query: string;
  libraries: DocsLibrary[];
}

export interface DocsFetchResult {
  library_id: string;
  topic?: string;
  content: string;
}

export interface SGraphResult {
  content: string;
}

const SEARCH_PROMPT_GUIDELINES = ["Use web_search to search the web for current facts."];
const FETCH_PROMPT_GUIDELINES = [
  "Use web_fetch to read a web page; large pages are truncated with a continuation notice, so follow up with tree or section_id to navigate.",
];
const DOCS_PROMPT_GUIDELINES = [
  "Use web_docs with action resolve, then action fetch, to read library documentation instead of fetching documentation sites page by page.",
  "For web_docs action fetch, provide the library_id returned by action resolve; use topic or tokens to narrow the result.",
];
const SGRAPH_PROMPT_GUIDELINES = [
  "Use web_sgraph to search public source code through Sourcegraph.",
];

function isRecord(input: unknown): input is Record<string, unknown> {
  return typeof input === "object" && input !== null;
}

function requireString(input: unknown, field: string): string {
  if (!isRecord(input) || typeof input[field] !== "string" || input[field].length === 0) {
    throw new Error(`${field} must be a non-empty string`);
  }
  return input[field] as string;
}

function requireQueries(input: unknown): string[] {
  if (!isRecord(input)) {
    throw new Error("queries must be an array of 1 to 4 non-empty strings");
  }
  for (const field of Object.keys(input)) {
    if (field !== "queries") {
      throw new Error(`web_search input does not accept field ${field}`);
    }
  }
  const queries = input.queries;
  if (
    !Array.isArray(queries) ||
    queries.length < 1 ||
    queries.length > 4 ||
    queries.some((query) => typeof query !== "string" || query.trim().length === 0)
  ) {
    throw new Error("queries must be an array of 1 to 4 non-empty strings");
  }
  return [...new Set(queries as string[])];
}

function mergeSearchResults(responses: SearchResult[]): SearchResult {
  const results: SearchResult["results"] = [];
  for (let resultIndex = 0; ; resultIndex++) {
    let added = false;
    for (const response of responses) {
      const result = response.results[resultIndex];
      if (result === undefined) continue;
      results.push({ ...result, position: results.length + 1 });
      added = true;
    }
    if (!added) break;
  }
  return { provider: responses[0]?.provider ?? "", results };
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

export function webSearchTool() {
  const binary = resolveBinaryPath("web", { require });
  return makeTool({
    name: "web_search",
    label: "Web search",
    description:
      "Search the web for current facts through the native Organon web search backends. Provide one to four queries; text output is limited to 2,000 lines or 50KB.",
    promptSnippet: "Search the web with web_search",
    promptGuidelines: SEARCH_PROMPT_GUIDELINES,
    parameters: webSearchSchema,
    execute: async (params: WebSearchInput, signal) => {
      const queries = requireQueries(params);
      const settled = await Promise.allSettled(
        queries.map(async (query) => {
          const result = await runCli(binary, { args: ["search", query, "--json"], signal });
          if (result.exitCode !== 0) throw await cliError(result.stderr, result.exitCode);
          return parseSingleJsonDoc<SearchResult>(result.stdout);
        }),
      );
      const responses = settled.map((result) => {
        if (result.status !== "fulfilled") throw result.reason;
        return result.value;
      });
      const data = mergeSearchResults(responses);
      const lines = data.results.map(
        (r) => `${r.position}. ${r.title}\n   URL: ${r.link}\n   ${r.snippet}`,
      );
      const raw =
        lines.length === 0
          ? "No search results."
          : `Found ${lines.length} search results (provider: ${data.provider}):\n\n` +
            lines.join("\n\n");
      return modelTextResult(data, raw, { hint: "Use a narrower search query to reduce results." });
    },
  });
}

export function webFetchTool() {
  return makeTool({
    name: "web_fetch",
    label: "Web fetch",
    description:
      "Fetch and read an HTTP or HTTPS web page as Markdown, with heading-tree navigation. Text output is limited to 2,000 lines or 50KB; truncated output is saved to a temporary file.",
    promptSnippet: "Fetch a web page with web_fetch",
    promptGuidelines: FETCH_PROMPT_GUIDELINES,
    parameters: webFetchSchema,
    execute: async (params: WebFetchInput, signal) => {
      const data = await fetchWebPage(
        {
          url: requireString(params, "url"),
          tree: params.tree,
          section_id: params.section_id,
          full: params.full,
          tree_threshold: params.tree_threshold,
        },
        signal,
      );
      return modelTextResult(data, data.content, {
        hint: "Use web_fetch with tree or section_id to navigate the document.",
      });
    },
  });
}

export function webDocsTool() {
  const binary = resolveBinaryPath("web", { require });
  return makeTool({
    name: "web_docs",
    label: "Web docs",
    description:
      "Resolve a library and fetch its documentation through Context7. Use action resolve with query, then action fetch with library_id; text output is limited to 2,000 lines or 50KB.",
    promptSnippet: "Resolve or fetch library documentation with web_docs",
    promptGuidelines: DOCS_PROMPT_GUIDELINES,
    parameters: webDocsSchema,
    execute: async (params: WebDocsInput, signal) => {
      const input = normalizeDocs(params);
      const args =
        input.action === "resolve"
          ? ["docs", "resolve", input.query, "--json"]
          : [
              "docs",
              "fetch",
              input.library_id,
              ...(input.topic !== undefined && input.topic !== "" ? [input.topic] : []),
              ...(input.tokens !== undefined && input.tokens !== 0
                ? ["--tokens", String(input.tokens)]
                : []),
              "--json",
            ];
      const result = await runCli(binary, { args, signal });
      if (result.exitCode !== 0) throw await cliError(result.stderr, result.exitCode);
      if (input.action === "resolve") {
        const data = parseSingleJsonDoc<DocsResolveResult>(result.stdout);
        const lines = data.libraries.map((lib) => `- ${lib.id}: ${lib.title}`);
        const raw =
          lines.length === 0
            ? `No libraries found for ${JSON.stringify(input.query)}`
            : `Found ${lines.length} libraries:\n` + lines.join("\n");
        return modelTextResult(data, raw, {
          hint: "Use web_docs action fetch with a library_id from the results.",
        });
      }
      const data = parseSingleJsonDoc<DocsFetchResult>(result.stdout);
      return modelTextResult(data, data.content, {
        hint: "Refetch with a narrower topic or tokens budget for the remainder.",
      });
    },
  });
}

export function webSgraphTool() {
  const binary = resolveBinaryPath("web", { require });
  return makeTool({
    name: "web_sgraph",
    label: "Web source search",
    description:
      "Search public source code through Sourcegraph. Text output is limited to 2,000 lines or 50KB; truncated output is saved to a temporary file.",
    promptSnippet: "Search public source code with web_sgraph",
    promptGuidelines: SGRAPH_PROMPT_GUIDELINES,
    parameters: webSgraphSchema,
    execute: async (params: WebSgraphInput, signal) => {
      const query = requireString(params, "query");
      const args = ["sgraph", query, "--json"];
      if (params.count !== undefined && params.count !== 10)
        args.push("--count", String(params.count));
      if (params.context !== undefined && params.context !== 10)
        args.push("--context", String(params.context));
      if (params.timeout !== undefined && params.timeout !== 0)
        args.push("--timeout", String(params.timeout));
      const result = await runCli(binary, { args, signal });
      if (result.exitCode !== 0) throw await cliError(result.stderr, result.exitCode);
      const data = parseSingleJsonDoc<SGraphResult>(result.stdout);
      return modelTextResult(data, data.content, {
        hint: "Use a narrower Sourcegraph query or lower count and context.",
      });
    },
  });
}

type ToolResult = Promise<{ content: { type: "text"; text: string }[]; details: unknown }>;

function makeTool<T extends object>(options: {
  name: string;
  label: string;
  description: string;
  promptSnippet: string;
  promptGuidelines: string[];
  parameters: T;
  execute(params: any, signal: AbortSignal | undefined): ToolResult;
}) {
  detectPlatform();
  return {
    name: options.name,
    label: options.label,
    description: options.description,
    promptSnippet: options.promptSnippet,
    promptGuidelines: options.promptGuidelines,
    parameters: options.parameters,
    async execute(
      _toolCallId: string,
      params: unknown,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      _ctx: unknown,
    ): ToolResult {
      return options.execute(params, signal);
    },
  };
}

export function registerWebTools(pi: { registerTool(definition: any): void }): void {
  pi.registerTool(webSearchTool());
  pi.registerTool(webFetchTool());
  pi.registerTool(webDocsTool());
  pi.registerTool(webSgraphTool());
}
