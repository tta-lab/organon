import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import { Type, type Static, type TSchema } from "typebox";

import {
  cliError,
  modelTextResult,
  parseSingleJsonDoc,
  resolveBinaryPath,
  runCli,
} from "@tta-lab/pi-shared";

const require = createRequire(import.meta.url);

const projectDesc =
  "Project reference: canonical alias, checkout basename, or remote repository basename";
const requiredProject = Type.String({ description: projectDesc, minLength: 1 });
const prIdDesc = "Optional positive PR ID; omitted uses the registered checkout's current branch";

const positivePrId = Type.Integer({
  description: prIdDesc,
  minimum: 1,
});

const nonBlankPrTitle = Type.String({
  description: "Non-blank pull request title",
  minLength: 1,
  pattern: "\\S",
});
export const ogCloneSchema = Type.Object(
  {
    project: Type.Optional(requiredProject),
    url: Type.Optional(
      Type.String({
        description: "HTTP(S) repository URL with exactly owner/repo",
        minLength: 1,
      }),
    ),
    alias: Type.Optional(Type.String({ description: "Optional exact single-layer project alias" })),
    reference: Type.Optional(
      Type.Boolean({
        description: "Clone under the references tree without registration",
        default: false,
      }),
    ),
  },
  {
    additionalProperties: false,
    description:
      "Provide exactly one of project or url. URL clones may also use alias or reference; project clones may not.",
  },
);

export const ogPullSchema = Type.Object(
  {
    project: requiredProject,
  },
  { additionalProperties: false },
);

export const ogPushSchema = Type.Object(
  {
    project: requiredProject,
    force: Type.Optional(
      Type.Boolean({
        description: "Use force-with-lease; rejected on the default branch",
        default: false,
      }),
    ),
  },
  { additionalProperties: false },
);

export const ogPrSchema = Type.Object(
  {
    action: StringEnum(["create", "find", "get", "modify", "comment"] as const, {
      description:
        "Pull request operation. create requires title; modify requires title or body; comment requires a non-blank body.",
    }),
    project: requiredProject,
    pr_id: Type.Optional(positivePrId),
    state: Type.Optional(
      StringEnum(["open", "closed", "all"] as const, {
        description: "Pull request state for find; defaults to open",
        default: "open",
      }),
    ),
    title: Type.Optional(nonBlankPrTitle),
    body: Type.Optional(
      Type.String({ description: "Pull request body or comment (may be multiline)" }),
    ),
  },
  { additionalProperties: false },
);

export const ogChecksSchema = Type.Object(
  {
    action: StringEnum(["status", "log", "failures"] as const, {
      description: "CI inspection operation; log and failures accept tail",
    }),
    project: requiredProject,
    pr_id: Type.Optional(positivePrId),
    tail: Type.Optional(
      Type.Integer({
        description: "Number of log or failure-log tail lines; defaults to 50",
        minimum: 0,
        maximum: 1000,
        default: 50,
      }),
    ),
  },
  { additionalProperties: false },
);

export type OgCloneInput = Static<typeof ogCloneSchema>;
export type OgPullInput = Static<typeof ogPullSchema>;
export type OgPushInput = Static<typeof ogPushSchema>;
export type OgPrInput = Static<typeof ogPrSchema>;
export type OgChecksInput = Static<typeof ogChecksSchema>;

type OgOperation =
  | { action: "pull"; project: string }
  | { action: "push"; project: string; force?: boolean }
  | { action: "clone"; project: string }
  | { action: "clone"; url: string; alias?: string; reference?: boolean }
  | { action: "pr_create"; project: string; title: string; body?: string }
  | { action: "pr_find"; project: string; state?: "open" | "closed" | "all" }
  | { action: "pr_get"; project: string; pr_id?: number }
  | { action: "pr_checks"; project: string; pr_id?: number }
  | {
      action: "pr_modify";
      project: string;
      pr_id?: number;
      title?: string;
      body?: string;
    }
  | { action: "pr_comment"; project: string; pr_id?: number; body: string }
  | { action: "pr_log"; project: string; pr_id?: number; tail?: number }
  | { action: "pr_failures"; project: string; pr_id?: number; tail?: number };

interface PullRequest {
  index: number;
  number?: number;
  title: string;
  state: string;
  merged: boolean;
  url: string;
  head: string;
  base: string;
  body: string;
  head_sha?: string;
}

interface Comment {
  id: number;
  pr_id: number;
  body: string;
  url: string;
}

interface CloneResult {
  alias?: string;
  path: string;
  host: string;
  owner: string;
  repo: string;
  provider: string;
  remote: string;
  registered: boolean;
  archived?: boolean;
  already_existed?: boolean;
}

const CLONE_PROMPT_GUIDELINES = [
  "Use og_clone with either a registered project reference or an HTTP(S) repository URL without embedded credentials; project clones never accept alias or reference selectors.",
  "Never pass paths, tokens, credential environment names, or credential-bearing URLs to og_clone.",
];

const PULL_PROMPT_GUIDELINES = [
  "Use og_pull with a project reference (canonical alias, checkout basename, or remote repository basename); never pass paths, tokens, or credential environment names.",
];

const PUSH_PROMPT_GUIDELINES = [
  "Use og_push with a project reference; force means force-with-lease and OG rejects it on the default branch. Never pass paths, tokens, or credential environment names.",
];

const PR_PROMPT_GUIDELINES = [
  "Use og_pr with a project reference and one of create, find, get, modify, or comment; never pass paths, tokens, or credential environment names.",
  "Use og_pr with action get without pr_id for the registered checkout's current-branch pull request, or with a positive pr_id for a branch-free remote operation.",
  "Use og_pr with action modify with at least one of title or body; an empty body explicitly clears it. Use action comment with a non-blank body.",
];

const CHECKS_PROMPT_GUIDELINES = [
  "Use og_checks with a project reference and one of status, log, or failures; never pass paths, tokens, or credential environment names.",
  "Use og_checks with action log or failures and an optional tail between 0 and 1000 lines to inspect CI output.",
];

export function ogCloneTool() {
  return makeOgTool({
    name: "og_clone",
    description:
      "Clone a registered project or an HTTP(S) repository URL with optional alias and reference modes. All calls use the package-local og binary, which owns credentials and policy.",
    promptSnippet: "Clone through the guarded package-local og binary with og_clone",
    promptGuidelines: CLONE_PROMPT_GUIDELINES,
    parameters: ogCloneSchema,
    normalize: (input) => normalizeClone(input as OgCloneInput),
  });
}

export function ogPullTool() {
  return makeOgTool({
    name: "og_pull",
    description:
      "Pull a registered project with guarded Git synchronization. All calls use the package-local og binary, which owns credentials and policy.",
    promptSnippet: "Pull a registered project through guarded OG with og_pull",
    promptGuidelines: PULL_PROMPT_GUIDELINES,
    parameters: ogPullSchema,
    normalize: (input) => normalizePull(input as OgPullInput),
  });
}

export function ogPushTool() {
  return makeOgTool({
    name: "og_push",
    description:
      "Push a registered project with guarded Git synchronization and optional force-with-lease. All calls use the package-local og binary, which owns credentials and policy.",
    promptSnippet: "Push a registered project through guarded OG with og_push",
    promptGuidelines: PUSH_PROMPT_GUIDELINES,
    parameters: ogPushSchema,
    normalize: (input) => normalizePush(input as OgPushInput),
  });
}

export function ogPrTool() {
  return makeOgTool({
    name: "og_pr",
    description:
      "Manage pull requests: create, find, get, modify, and comment through guarded OG operations. All calls use the package-local og binary, which owns credentials and policy.",
    promptSnippet: "Manage pull requests through guarded OG with og_pr",
    promptGuidelines: PR_PROMPT_GUIDELINES,
    parameters: ogPrSchema,
    normalize: (input) => normalizePr(input as OgPrInput),
  });
}

export function ogChecksTool() {
  return makeOgTool({
    name: "og_checks",
    description:
      "Inspect pull-request status, logs, and failures through guarded OG operations. All calls use the package-local og binary, which owns credentials and policy.",
    promptSnippet: "Inspect pull-request status and CI output through guarded OG with og_checks",
    promptGuidelines: CHECKS_PROMPT_GUIDELINES,
    parameters: ogChecksSchema,
    normalize: (input) => normalizeChecks(input as OgChecksInput),
  });
}

function record(input: unknown): Record<string, unknown> {
  if (typeof input !== "object" || input === null || Array.isArray(input)) {
    throw new Error("tool input must be an object");
  }
  return input as Record<string, unknown>;
}

function requireProject(input: unknown): string {
  const value = record(input).project;
  if (typeof value !== "string" || !/\S/.test(value)) {
    throw new Error("project reference must not be empty");
  }
  return value;
}

function rejectUnknownFields(input: Record<string, unknown>, allowed: string[]): void {
  for (const field of Object.keys(input)) {
    if (!allowed.includes(field)) {
      throw new Error(`unknown field ${field}`);
    }
  }
}

function rejectFields(input: Record<string, unknown>, action: string, fields: string[]): void {
  for (const field of fields) {
    if (field in input) {
      throw new Error(`og_${action} action does not accept field ${field}`);
    }
  }
}

function requirePositivePrID(value: unknown): number | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "number" || !Number.isInteger(value) || value < 1) {
    throw new Error("pr_id must be a positive integer");
  }
  return value;
}

function requireNonBlank(value: unknown, field: string): string {
  if (typeof value !== "string" || !/\S/.test(value)) {
    throw new Error(`${field} must not be blank`);
  }
  return value;
}

function normalizeClone(input: OgCloneInput): OgOperation {
  const value = record(input);
  rejectUnknownFields(value, ["project", "url", "alias", "reference"]);
  const hasProject = "project" in value;
  const hasURL = "url" in value;
  if (hasProject === hasURL) {
    throw new Error("og_clone requires exactly one of project or url");
  }
  if (hasProject) {
    const project = requireProject(input);
    rejectFields(value, "clone", ["alias", "reference"]);
    return { action: "clone", project };
  }
  if (typeof value.url !== "string" || !/\S/.test(value.url)) {
    throw new Error("url must not be empty");
  }
  if (value.alias !== undefined && typeof value.alias !== "string") {
    throw new Error("alias must be a string");
  }
  if (value.reference !== undefined && typeof value.reference !== "boolean") {
    throw new Error("reference must be a boolean");
  }
  return {
    action: "clone",
    url: value.url,
    alias: value.alias as string | undefined,
    reference: value.reference as boolean | undefined,
  };
}

function normalizePull(input: OgPullInput): OgOperation {
  rejectUnknownFields(record(input), ["project"]);
  return { action: "pull", project: requireProject(input) };
}

function normalizePush(input: OgPushInput): OgOperation {
  const value = record(input);
  rejectUnknownFields(value, ["project", "force"]);
  if (value.force !== undefined && typeof value.force !== "boolean") {
    throw new Error("force must be a boolean");
  }
  return {
    action: "push",
    project: requireProject(input),
    force: value.force as boolean | undefined,
  };
}

function normalizePr(input: OgPrInput): OgOperation {
  const value = record(input);
  rejectUnknownFields(value, ["action", "project", "pr_id", "state", "title", "body"]);
  if (typeof value.action !== "string") throw new Error("og_pr action is required");
  const project = requireProject(input);
  const pr_id = requirePositivePrID(value.pr_id);
  switch (value.action) {
    case "create":
      rejectFields(value, "pr", ["pr_id", "state"]);
      if (value.body !== undefined && typeof value.body !== "string") {
        throw new Error("body must be a string");
      }
      return {
        action: "pr_create",
        project,
        title: requireNonBlank(value.title, "title"),
        body: value.body as string | undefined,
      };
    case "find":
      rejectFields(value, "pr", ["pr_id", "title", "body"]);
      if (
        value.state !== undefined &&
        value.state !== "open" &&
        value.state !== "closed" &&
        value.state !== "all"
      ) {
        throw new Error("state must be open, closed, or all");
      }
      return {
        action: "pr_find",
        project,
        state: value.state as "open" | "closed" | "all" | undefined,
      };
    case "get":
      rejectFields(value, "pr", ["state", "title", "body"]);
      return { action: "pr_get", project, pr_id };
    case "modify":
      rejectFields(value, "pr", ["state"]);
      if (value.title === undefined && value.body === undefined) {
        throw new Error("pull request modify requires title or body; nothing to update");
      }
      if (value.title !== undefined) requireNonBlank(value.title, "title");
      if (value.body !== undefined && typeof value.body !== "string") {
        throw new Error("body must be a string");
      }
      return {
        action: "pr_modify",
        project,
        pr_id,
        title: value.title as string | undefined,
        body: value.body as string | undefined,
      };
    case "comment":
      rejectFields(value, "pr", ["state", "title"]);
      return {
        action: "pr_comment",
        project,
        pr_id,
        body: requireNonBlank(value.body, "comment body"),
      };
    default:
      throw new Error("og_pr action must be create, find, get, modify, or comment");
  }
}

function normalizeChecks(input: OgChecksInput): OgOperation {
  const value = record(input);
  rejectUnknownFields(value, ["action", "project", "pr_id", "tail"]);
  if (typeof value.action !== "string") throw new Error("og_checks action is required");
  const project = requireProject(input);
  const pr_id = requirePositivePrID(value.pr_id);
  if (
    value.tail !== undefined &&
    (typeof value.tail !== "number" ||
      !Number.isInteger(value.tail) ||
      value.tail < 0 ||
      value.tail > 1000)
  ) {
    throw new Error("tail must be an integer between 0 and 1000");
  }
  switch (value.action) {
    case "status":
      rejectFields(value, "checks", ["tail"]);
      return { action: "pr_checks", project, pr_id };
    case "log":
      return { action: "pr_log", project, pr_id, tail: value.tail as number | undefined };
    case "failures":
      return { action: "pr_failures", project, pr_id, tail: value.tail as number | undefined };
    default:
      throw new Error("og_checks action must be status, log, or failures");
  }
}

function makeOgTool(options: {
  name: string;
  description: string;
  promptSnippet: string;
  promptGuidelines: string[];
  parameters: TSchema;
  normalize(input: unknown): OgOperation;
}) {
  const binary = resolveBinaryPath("og", { require });
  return {
    name: options.name,
    label: options.name,
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
    ): Promise<{ content: { type: "text"; text: string }[]; details: unknown }> {
      const operation = options.normalize(params);
      const { args, stdin } = buildArgs(operation);
      const result = await runCli(binary, { args, stdin, signal });
      if (result.exitCode !== 0) {
        throw await cliError(result.stderr, result.exitCode);
      }
      return await render(operation, result.stdout);
    },
  };
}

function isAction<T extends OgOperation["action"]>(
  input: OgOperation,
  action: T,
): input is Extract<OgOperation, { action: T }> {
  return input.action === action;
}

function buildArgs(input: OgOperation): { args: string[]; stdin?: string } {
  const project = projectArgs(input);
  if (isAction(input, "pull")) {
    return { args: ["pull", ...project, "--json"] };
  }
  if (isAction(input, "push")) {
    return { args: ["push", ...project, ...(input.force ? ["--force"] : []), "--json"] };
  }
  if (isAction(input, "clone")) {
    // Runtime validation enforces exactly one selector mode; the CLI enforces
    // the remaining repository URL domain rules.
    const projectClone = "project" in input;
    const args = ["clone", projectClone ? input.project : input.url];
    if (!projectClone && input.alias) {
      args.push("--alias", input.alias);
    }
    if (!projectClone && input.reference) {
      args.push("--reference");
    }
    args.push("--json");
    return { args };
  }
  if (isAction(input, "pr_create")) {
    const args = ["pr", "create", input.title, ...project, "--json"];
    return { args, stdin: input.body ?? "" };
  }
  if (isAction(input, "pr_find")) {
    const args = ["pr", "find", ...project];
    if (input.state && input.state !== "open") {
      args.push("--state", input.state);
    }
    args.push("--json");
    return { args };
  }
  if (isAction(input, "pr_get")) {
    if (input.pr_id !== undefined) {
      return { args: ["pr", "get", String(input.pr_id), ...project, "--json"] };
    }
    return { args: ["pr", "view", ...project, "--json"] };
  }
  if (isAction(input, "pr_checks")) {
    return { args: ["pr", "checks", ...project, ...prIdArgs(input), "--json"] };
  }
  if (isAction(input, "pr_modify")) {
    const args = ["pr", "modify", ...project, ...prIdArgs(input)];
    if (input.title !== undefined) {
      args.push("--title", input.title);
    }
    if (input.body === "") {
      args.push("--clear-body");
    }
    args.push("--json");
    return { args, stdin: input.body ?? "" };
  }
  if (isAction(input, "pr_comment")) {
    const args = ["pr", "comment", ...project, ...prIdArgs(input), "--json"];
    return { args, stdin: input.body };
  }
  const args = [
    "pr",
    input.action === "pr_failures" ? "failures" : "log",
    ...project,
    ...prIdArgs(input),
  ];
  if (input.tail !== undefined && input.tail !== 50) {
    args.push("--tail", String(input.tail));
  }
  args.push("--json");
  return { args };
}

function projectArgs(input: OgOperation): string[] {
  if (!("project" in input)) {
    return [];
  }
  if (input.project === "") {
    throw new Error("project reference must not be empty");
  }
  return ["--project", input.project];
}

function prIdArgs(input: { pr_id?: number }): string[] {
  return input.pr_id !== undefined ? ["--pr-id", String(input.pr_id)] : [];
}

async function render(
  input: OgOperation,
  stdout: string,
): Promise<{ content: { type: "text"; text: string }[]; details: unknown }> {
  if (isAction(input, "push") || isAction(input, "pull")) {
    const data = parseSingleJsonDoc<{ project?: string; message: string }>(stdout);
    return modelTextResult(data, `${data.project ?? ""}: ${data.message}`.replace(/^: /, ""), {
      hint: useHint(input, "to inspect the operation again."),
    });
  }
  if (isAction(input, "clone")) {
    const data = parseSingleJsonDoc<{ clone: CloneResult }>(stdout);
    const c = data.clone;
    const text = c.registered ? `Cloned ${c.alias} to ${c.path}` : `Cloned reference to ${c.path}`;
    return modelTextResult(data, text, {
      hint: useHint(input, "again only if another clone is needed."),
    });
  }
  if (isAction(input, "pr_comment")) {
    const data = parseSingleJsonDoc<{ project?: string; comment: Comment }>(stdout);
    const text = `${data.project ? `${data.project}: ` : ""}Commented on PR #${data.comment.pr_id}: ${data.comment.url}`;
    return modelTextResult(data, text, {
      hint: useHint(input, "to add a follow-up comment."),
    });
  }
  if (isAction(input, "pr_log") || isAction(input, "pr_failures") || isAction(input, "pr_checks")) {
    const data = parseSingleJsonDoc<{ project?: string; pr: PullRequest; lines: string[] }>(stdout);
    const pr = formatPR(data.pr);
    const rendered = `${data.project ? `${data.project}: ` : ""}${pr}`;
    const text = data.lines.length ? `${rendered}\n\n${data.lines.join("\n")}` : rendered;
    const hint = isAction(input, "pr_checks")
      ? useHint(input, "to inspect checks again.")
      : useHint(input, "with a smaller tail to narrow logs.");
    return modelTextResult(data, text, { hint });
  }
  const data = parseSingleJsonDoc<{ project?: string; pr: PullRequest }>(stdout);
  return modelTextResult(data, `${data.project ? `${data.project}: ` : ""}${formatPR(data.pr)}`, {
    hint: useHint(input, "to inspect the pull request again."),
  });
}

const PUBLIC_OPERATION_TARGETS: Record<OgOperation["action"], { tool: string; action?: string }> = {
  clone: { tool: "og_clone" },
  pull: { tool: "og_pull" },
  push: { tool: "og_push" },
  pr_create: { tool: "og_pr", action: "create" },
  pr_find: { tool: "og_pr", action: "find" },
  pr_get: { tool: "og_pr", action: "get" },
  pr_modify: { tool: "og_pr", action: "modify" },
  pr_comment: { tool: "og_pr", action: "comment" },
  pr_checks: { tool: "og_checks", action: "status" },
  pr_log: { tool: "og_checks", action: "log" },
  pr_failures: { tool: "og_checks", action: "failures" },
};

function useHint(input: OgOperation, suffix: string): string {
  const target = PUBLIC_OPERATION_TARGETS[input.action];
  return `Use ${target.tool}${target.action ? ` with action ${target.action}` : ""} ${suffix}`;
}

function formatPR(pr: PullRequest): string {
  let text = `PR #${pr.index} ${pr.title} [${pr.state}]${pr.url ? `\n${pr.url}` : ""}`;
  if (pr.body) {
    text += `\n\n${pr.body}`;
  }
  return text;
}

type OgToolDefinition = ReturnType<typeof makeOgTool>;

export function registerOgTools(pi: { registerTool(definition: OgToolDefinition): void }): void {
  pi.registerTool(ogCloneTool());
  pi.registerTool(ogPullTool());
  pi.registerTool(ogPushTool());
  pi.registerTool(ogPrTool());
  pi.registerTool(ogChecksTool());
}
