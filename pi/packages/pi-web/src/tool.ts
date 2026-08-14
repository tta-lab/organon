import { createRequire } from "node:module";

import { formatSize, truncateHead, type TruncationResult } from "@earendil-works/pi-coding-agent";
import { Type, type Static } from "typebox";

import { cliError, parseSingleJsonDoc, resolveBinaryPath, runCli } from "@tta-lab/pi-shared";

const require = createRequire(import.meta.url);

const DEFAULT_TREE_THRESHOLD = 5000;

export const webSchema = Type.Union([
  Type.Object(
    {
      action: Type.Literal("search"),
      query: Type.String({ description: "Web search query" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("fetch"),
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
          minimum: 0,
        }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("docs_resolve"),
      query: Type.String({ description: "Library name or package query" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("docs_fetch"),
      library_id: Type.String({ description: "Context7 library ID returned by docs_resolve" }),
      topic: Type.Optional(Type.String({ description: "Optional documentation topic" })),
      tokens: Type.Optional(
        Type.Integer({
          description: "Optional token budget; zero uses the backend default",
          default: 0,
          minimum: 0,
        }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("sgraph"),
      query: Type.String({ description: "Sourcegraph search query" }),
      count: Type.Optional(
        Type.Integer({
          description: "Optional result count; defaults to 10",
          default: 10,
          minimum: 0,
        }),
      ),
      context: Type.Optional(
        Type.Integer({
          description: "Optional context lines; defaults to 10",
          default: 10,
          minimum: 0,
        }),
      ),
      timeout: Type.Optional(
        Type.Integer({
          description: "Optional timeout in seconds; zero disables the timeout",
          default: 0,
          minimum: 0,
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

export interface FetchResult {
  url: string;
  mode: string;
  content: string;
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
export function truncateForModel(content: string): {
  text: string;
  truncation?: TruncationResult;
} {
  const truncation = truncateHead(content);
  if (!truncation.truncated) {
    return { text: content };
  }
  let text: string;
  if (truncation.firstLineExceedsLimit) {
    text = `[First line is ${formatSize(truncation.totalBytes)}, exceeds ${formatSize(50 * 1024)} limit]`;
  } else if (truncation.truncatedBy === "lines") {
    text =
      truncation.content +
      `\n\n[Truncated: showing ${truncation.outputLines} of ${truncation.totalLines} lines ` +
      `(${truncation.maxLines} line limit). Use fetch with tree or section_id to navigate the document.]`;
  } else {
    text =
      truncation.content +
      `\n\n[Truncated: ${truncation.outputLines} lines shown (${formatSize(truncation.maxBytes)} limit). ` +
      `Use fetch with tree or section_id to navigate the document.]`;
  }
  return { text, truncation };
}

export function webTool() {
  return {
    name: "web",
    label: "Web",
    description:
      "Search the web, fetch and read web pages as Markdown, resolve and fetch library documentation, " +
      "and search public source code through Sourcegraph.",
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
      const binary = resolveBinaryPath("web", { require });
      const args = buildArgs(params);
      const result = await runCli(binary, { args, signal });
      if (result.exitCode !== 0) {
        throw cliError(result.stderr, result.exitCode);
      }
      return render(params, result.stdout);
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

function render(
  input: WebInput,
  stdout: string,
): { content: { type: "text"; text: string }[]; details: unknown } {
  if (isAction(input, "search")) {
    const data = parseSingleJsonDoc<SearchResult>(stdout);
    const lines = data.results.map(
      (r) => `${r.position}. ${r.title}\n   URL: ${r.link}\n   ${r.snippet}`,
    );
    const text =
      lines.length === 0
        ? "No search results."
        : `Found ${lines.length} search results (provider: ${data.provider}):\n\n` +
          lines.join("\n\n");
    return { content: [{ type: "text", text }], details: data };
  }
  if (isAction(input, "fetch")) {
    const data = parseSingleJsonDoc<FetchResult>(stdout);
    const { text, truncation } = truncateForModel(data.content);
    return { content: [{ type: "text", text }], details: { ...data, truncation } };
  }
  if (isAction(input, "docs_resolve")) {
    const data = parseSingleJsonDoc<DocsResolveResult>(stdout);
    const lines = data.libraries.map((lib) => `- ${lib.id}: ${lib.title}`);
    const text =
      lines.length === 0
        ? `No libraries found for ${JSON.stringify(input.query)}`
        : `Found ${lines.length} libraries:\n` + lines.join("\n");
    return { content: [{ type: "text", text }], details: data };
  }
  if (isAction(input, "docs_fetch")) {
    const data = parseSingleJsonDoc<DocsFetchResult>(stdout);
    const { text, truncation } = truncateForModel(data.content);
    return { content: [{ type: "text", text }], details: { ...data, truncation } };
  }
  const data = parseSingleJsonDoc<SGraphResult>(stdout);
  const { text, truncation } = truncateForModel(data.content);
  return { content: [{ type: "text", text }], details: { ...data, truncation } };
}

export function registerWebTool(pi: {
  registerTool(definition: ReturnType<typeof webTool>): void;
}): void {
  pi.registerTool(webTool());
}
