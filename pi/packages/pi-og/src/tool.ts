import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import { Type, type Static, type TSchema } from "typebox";

import {
  objectUnion,
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
const nonBlankCommentBody = Type.String({
  description: "Non-blank pull request comment body (may be multiline)",
  minLength: 1,
  pattern: "\\S",
});

export const ogAuthStatusSchema = Type.Object(
  {
    project: requiredProject,
  },
  { additionalProperties: false },
);

export const ogCloneSchema = objectUnion([
  Type.Object(
    {
      project: requiredProject,
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      url: Type.String({
        description: "HTTP(S) repository URL with exactly owner/repo",
        minLength: 1,
      }),
      alias: Type.Optional(
        Type.String({ description: "Optional exact single-layer project alias" }),
      ),
      reference: Type.Optional(
        Type.Boolean({
          description: "Clone under the references tree without registration",
          default: false,
        }),
      ),
    },
    { additionalProperties: false },
  ),
]);

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

export const ogPrSchema = objectUnion([
  Type.Object(
    {
      action: StringEnum(["create"] as const, { description: "Pull request operation" }),
      project: requiredProject,
      title: nonBlankPrTitle,
      body: Type.Optional(
        Type.String({ description: "Optional pull request body (may be multiline)" }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["find"] as const, { description: "Pull request operation" }),
      project: requiredProject,
      state: Type.Optional(
        StringEnum(["open", "closed", "all"] as const, {
          description: "Pull request state to search",
          default: "open",
        }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["get"] as const, { description: "Pull request operation" }),
      project: requiredProject,
      pr_id: Type.Optional(positivePrId),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["modify"] as const, { description: "Pull request operation" }),
      project: requiredProject,
      pr_id: Type.Optional(positivePrId),
      title: nonBlankPrTitle,
      body: Type.Optional(
        Type.String({ description: "Replacement pull request body; an empty string clears it" }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["modify"] as const, { description: "Pull request operation" }),
      project: requiredProject,
      pr_id: Type.Optional(positivePrId),
      title: Type.Optional(nonBlankPrTitle),
      body: Type.String({
        description: "Replacement pull request body; an empty string clears it",
      }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["comment"] as const, { description: "Pull request operation" }),
      project: requiredProject,
      pr_id: Type.Optional(positivePrId),
      body: nonBlankCommentBody,
    },
    { additionalProperties: false },
  ),
]);

export const ogChecksSchema = objectUnion([
  Type.Object(
    {
      action: StringEnum(["status"] as const, { description: "CI inspection operation" }),
      project: requiredProject,
      pr_id: Type.Optional(positivePrId),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["log"] as const, { description: "CI inspection operation" }),
      project: requiredProject,
      pr_id: Type.Optional(positivePrId),
      tail: Type.Optional(
        Type.Integer({
          description: "Number of log tail lines; defaults to 50",
          minimum: 0,
          maximum: 1000,
          default: 50,
        }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["failures"] as const, { description: "CI inspection operation" }),
      project: requiredProject,
      pr_id: Type.Optional(positivePrId),
      tail: Type.Optional(
        Type.Integer({
          description: "Number of failure-log tail lines; defaults to 50",
          minimum: 0,
          maximum: 1000,
          default: 50,
        }),
      ),
    },
    { additionalProperties: false },
  ),
]);

export type OgAuthStatusInput = Static<typeof ogAuthStatusSchema>;
export type OgCloneInput = Static<typeof ogCloneSchema>;
export type OgPullInput = Static<typeof ogPullSchema>;
export type OgPushInput = Static<typeof ogPushSchema>;
export type OgPrInput = Static<typeof ogPrSchema>;
export type OgChecksInput = Static<typeof ogChecksSchema>;

type OgOperation =
  | { action: "auth_status"; project: string }
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

interface AuthStatus {
  project: string;
  provider: string;
  host: string;
  owner: string;
  repo: string;
  auth_mode: string;
  ready: boolean;
  token_env?: string;
  token_set?: boolean;
  permissions?: Array<{ name: string; required: string; actual?: string; ready: boolean }>;
}

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

const AUTH_PROMPT_GUIDELINES = [
  "Use og_auth_status with a project reference (canonical alias, checkout basename, or remote repository basename); never pass paths, tokens, or credential environment names.",
];

const CLONE_PROMPT_GUIDELINES = [
  "Use og_clone with either a registered project reference or an HTTP(S) repository URL without embedded credentials; project clones never accept alias or reference selectors.",
  "Never pass paths, tokens, credential environment names, or credential-bearing URLs to og_clone.",
];

const PULL_PROMPT_GUIDELINES = [
  "Use og_pull with a project reference (canonical alias, checkout basename, or remote repository basename); never pass paths, tokens, or credential environment names.",
];

const PUSH_PROMPT_GUIDELINES = [
  "Use og_push with a project reference; force means force-with-lease and OG rejects it on the default branch. Never pass paths, tokens, or credential environment names.",
  "Use og_auth_status to check forge authentication before retrying a failed og_push operation.",
];

const PR_PROMPT_GUIDELINES = [
  "Use og_pr with a project reference and one of create, find, get, modify, or comment; never pass paths, tokens, or credential environment names.",
  "Use og_auth_status to check forge authentication before retrying failed og_pr operations.",
  "Use og_pr with action get without pr_id for the registered checkout's current-branch pull request, or with a positive pr_id for a branch-free remote operation.",
  "Use og_pr with action modify with at least one of title or body; an empty body explicitly clears it. Use action comment with a non-blank body.",
];

const CHECKS_PROMPT_GUIDELINES = [
  "Use og_checks with a project reference and one of status, log, or failures; never pass paths, tokens, or credential environment names.",
  "Use og_checks with action log or failures and an optional tail between 0 and 1000 lines to inspect CI output.",
];

export function ogAuthStatusTool() {
  return makeOgTool({
    name: "og_auth_status",
    description:
      "Check guarded forge authentication for a registered project reference. All calls use the package-local og binary, which owns credentials and policy.",
    promptSnippet: "Check guarded forge authentication with og_auth_status",
    promptGuidelines: AUTH_PROMPT_GUIDELINES,
    parameters: ogAuthStatusSchema,
    normalize: (input) => normalizeAuthStatus(input as OgAuthStatusInput),
  });
}

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

function normalizeAuthStatus(input: OgAuthStatusInput): OgOperation {
  return { action: "auth_status", project: input.project };
}

function normalizeClone(input: OgCloneInput): OgOperation {
  if ("project" in input) {
    return { action: "clone", project: input.project };
  }
  return {
    action: "clone",
    url: input.url,
    alias: input.alias,
    reference: input.reference,
  };
}

function normalizePull(input: OgPullInput): OgOperation {
  return { action: "pull", project: input.project };
}

function normalizePush(input: OgPushInput): OgOperation {
  return { action: "push", project: input.project, force: input.force };
}

function normalizePr(input: OgPrInput): OgOperation {
  switch (input.action) {
    case "create":
      return { action: "pr_create", project: input.project, title: input.title, body: input.body };
    case "find":
      return { action: "pr_find", project: input.project, state: input.state };
    case "get":
      return { action: "pr_get", project: input.project, pr_id: input.pr_id };
    case "modify":
      return {
        action: "pr_modify",
        project: input.project,
        pr_id: input.pr_id,
        title: input.title,
        body: input.body,
      };
    case "comment":
      return {
        action: "pr_comment",
        project: input.project,
        pr_id: input.pr_id,
        body: input.body,
      };
  }
}

function normalizeChecks(input: OgChecksInput): OgOperation {
  switch (input.action) {
    case "status":
      return { action: "pr_checks", project: input.project, pr_id: input.pr_id };
    case "log":
      return {
        action: "pr_log",
        project: input.project,
        pr_id: input.pr_id,
        tail: input.tail,
      };
    case "failures":
      return {
        action: "pr_failures",
        project: input.project,
        pr_id: input.pr_id,
        tail: input.tail,
      };
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
  if (isAction(input, "auth_status")) {
    return { args: ["auth", "status", ...project, "--json"] };
  }
  if (isAction(input, "pull")) {
    return { args: ["pull", ...project, "--json"] };
  }
  if (isAction(input, "push")) {
    return { args: ["push", ...project, ...(input.force ? ["--force"] : []), "--json"] };
  }
  if (isAction(input, "clone")) {
    // The schema enforces exactly one selector mode; the CLI enforces the
    // remaining domain rules (project clones never accept alias/reference).
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
  if (isAction(input, "auth_status")) {
    const data = parseSingleJsonDoc<{ project?: string; auth: AuthStatus }>(stdout);
    const a = data.auth;
    const project = data.project ?? a.project;
    const text =
      `${project}: ${a.ready ? "Authenticated" : "Not authenticated"}: ${a.provider} ${a.host}/${a.owner}/${a.repo}` +
      (a.ready ? "" : ` (auth mode: ${a.auth_mode})`);
    return modelTextResult(data, text, {
      hint: useHint(input, "again after resolving the reported issue."),
    });
  }
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
  auth_status: { tool: "og_auth_status" },
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
  pi.registerTool(ogAuthStatusTool());
  pi.registerTool(ogCloneTool());
  pi.registerTool(ogPullTool());
  pi.registerTool(ogPushTool());
  pi.registerTool(ogPrTool());
  pi.registerTool(ogChecksTool());
}
