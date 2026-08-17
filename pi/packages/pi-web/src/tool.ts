import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import { Type, type Static } from "typebox";

import {
  objectUnion,
  cliError,
  detectPlatform,
  parseSingleJsonDoc,
  resolveBinaryPath,
  runCli,
  modelTextResult,
} from "@tta-lab/pi-shared";

import { fetchWebPage, type FetchResult } from "./fetch.js";
export type { FetchResult } from "./fetch.js";

const require = createRequire(import.meta.url);

const DEFAULT_TREE_THRESHOLD = 5000;

export const webSchema = objectUnion([
  Type.Object(
    {
      action: StringEnum(["search"] as const, { description: "Action to perform" }),
      query: Type.String({ description: "Web search query" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["fetch"] as const, { description: "Action to perform" }),
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
  ),
  Type.Object(
    {
      action: StringEnum(["docs_resolve"] as const, { description: "Action to perform" }),
      query: Type.String({ description: "Library name or package query" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["docs_fetch"] as const, { description: "Action to perform" }),
      library_id: Type.String({ description: "Context7 library ID returned by docs_resolve" }),
      topic: Type.Optional(Type.String({ description: "Optional documentation topic" })),
      tokens: Type.Optional(
        Type.Integer({
          description: "Optional token budget; zero uses the backend default",
          default: 0,
        }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["sgraph"] as const, { description: "Action to perform" }),
      query: Type.String({ description: "Sourcegraph search query" }),
      count: Type.Optional(
        Type.Integer({
          description: "Optional result count; defaults to 10",
          default: 10,
        }),
      ),
      context: Type.Optional(
        Type.Integer({
          description: "Optional context lines; defaults to 10",
          default: 10,
        }),
      ),
      timeout: Type.Optional(
        Type.Integer({
          description: "Optional timeout in seconds; zero disables the timeout",
          default: 0,
        }),
      ),
    },
    { additionalProperties: false },
  ),
]);

export type WebInput = Static<typeof webSchema>;

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

const WEB_PROMPT_GUIDELINES = [
  "Use web with action search to search the web for current facts.",
  "Use web with action fetch to read a web page; large pages are truncated with a continuation notice, so follow up with tree or section_id to navigate.",
  "Use web with action docs_resolve then docs_fetch to read library documentation instead of fetching documentation sites page by page.",
  "Use web with action sgraph to search public source code through Sourcegraph.",
];

type Action =
  | { action: "search"; query: string }
  | {
      action: "fetch";
      url: string;
      tree?: boolean;
      section_id?: string;
      full?: boolean;
      tree_threshold?: number;
    }
  | { action: "docs_resolve"; query: string }
  | { action: "docs_fetch"; library_id: string; topic?: string; tokens?: number }
  | { action: "sgraph"; query: string; count?: number; context?: number; timeout?: number };

function isAction<T extends Action["action"]>(
  input: WebInput,
  action: T,
): input is Extract<WebInput, { action: T }> {
  return input.action === action;
}

/**
 * Truncates model-facing text per Pi's 2,000-line / 50-KB contract and returns
 * an actionable continuation notice. The full structured result stays in
 * tool-result details.
 */
export function webTool() {
  detectPlatform();
  return {
    name: "web",
    label: "Web",
    description:
      "Search the web, fetch and read web pages as Markdown, resolve and fetch library documentation, " +
      "and search public source code through Sourcegraph. Text output is limited to 2,000 lines or 50KB; " +
      "truncated output is saved to a temporary file.",
    promptSnippet: "Search the web, fetch pages, read docs, and search source code",
    promptGuidelines: WEB_PROMPT_GUIDELINES,
    parameters: webSchema,
    async execute(
      _toolCallId: string,
      params: WebInput,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      _ctx: unknown,
    ): Promise<{ content: { type: "text"; text: string }[]; details: unknown }> {
      if (isAction(params, "fetch")) {
        const data = await fetchWebPage(params, signal);
        return await modelTextResult(data, data.content, {
          hint: "Use fetch with tree or section_id to navigate the document.",
        });
      }

      const binary = resolveBinaryPath("web", { require });
      const args = buildArgs(params);
      const result = await runCli(binary, { args, signal });
      if (result.exitCode !== 0) {
        throw await cliError(result.stderr, result.exitCode);
      }
      return await render(params, result.stdout);
    },
  };
}

function buildArgs(input: WebInput): string[] {
  if (isAction(input, "search")) {
    return ["search", input.query, "--json"];
  }
  if (isAction(input, "fetch")) {
    const args = ["fetch", input.url, "--json"];
    if (input.tree) {
      args.push("--tree");
    }
    if (input.section_id) {
      args.push("--section-id", input.section_id);
    }
    if (input.full) {
      args.push("--full");
    }
    if (input.tree_threshold !== undefined && input.tree_threshold !== DEFAULT_TREE_THRESHOLD) {
      args.push("--tree-threshold", String(input.tree_threshold));
    }
    return args;
  }
  if (isAction(input, "docs_resolve")) {
    return ["docs", "resolve", input.query, "--json"];
  }
  if (isAction(input, "docs_fetch")) {
    const args = ["docs", "fetch", input.library_id];
    if (input.topic !== undefined && input.topic !== "") {
      args.push(input.topic);
    }
    if (input.tokens !== undefined && input.tokens !== 0) {
      args.push("--tokens", String(input.tokens));
    }
    args.push("--json");
    return args;
  }
  const args = ["sgraph", input.query, "--json"];
  if (input.count !== undefined && input.count !== 10) {
    args.push("--count", String(input.count));
  }
  if (input.context !== undefined && input.context !== 10) {
    args.push("--context", String(input.context));
  }
  if (input.timeout !== undefined && input.timeout !== 0) {
    args.push("--timeout", String(input.timeout));
  }
  return args;
}

async function render(
  input: WebInput,
  stdout: string,
): Promise<{ content: { type: "text"; text: string }[]; details: unknown }> {
  if (isAction(input, "search")) {
    const data = parseSingleJsonDoc<SearchResult>(stdout);
    const lines = data.results.map(
      (r) => `${r.position}. ${r.title}\n   URL: ${r.link}\n   ${r.snippet}`,
    );
    const raw =
      lines.length === 0
        ? "No search results."
        : `Found ${lines.length} search results (provider: ${data.provider}):\n\n` +
          lines.join("\n\n");
    return modelTextResult(data, raw, { hint: "Use a narrower search query to reduce results." });
  }
  if (isAction(input, "fetch")) {
    const data = parseSingleJsonDoc<FetchResult>(stdout);
    return modelTextResult(data, data.content, {
      hint: "Use fetch with tree or section_id to navigate the document.",
    });
  }
  if (isAction(input, "docs_resolve")) {
    const data = parseSingleJsonDoc<DocsResolveResult>(stdout);
    const lines = data.libraries.map((lib) => `- ${lib.id}: ${lib.title}`);
    const raw =
      lines.length === 0
        ? `No libraries found for ${JSON.stringify(input.query)}`
        : `Found ${lines.length} libraries:\n` + lines.join("\n");
    return modelTextResult(data, raw, {
      hint: "Use a more specific library query to narrow results.",
    });
  }
  if (isAction(input, "docs_fetch")) {
    const data = parseSingleJsonDoc<DocsFetchResult>(stdout);
    return modelTextResult(data, data.content, {
      hint: "Refetch with a narrower topic or tokens budget for the remainder.",
    });
  }
  const data = parseSingleJsonDoc<SGraphResult>(stdout);
  return modelTextResult(data, data.content, {
    hint: "Use a narrower Sourcegraph query or lower count and context.",
  });
}

export function registerWebTool(pi: {
  registerTool(definition: ReturnType<typeof webTool>): void;
}): void {
  pi.registerTool(webTool());
}
