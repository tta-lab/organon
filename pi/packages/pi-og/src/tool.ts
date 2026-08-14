import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import { Type, type Static } from "typebox";

import { cliError, parseSingleJsonDoc, resolveBinaryPath, runCli } from "@tta-lab/pi-shared";

const require = createRequire(import.meta.url);

const projectDesc = "Exact registered single-layer project alias";
const prIdDesc = "Optional positive PR ID; omitted uses the registered checkout's current branch";

const positivePrId = Type.Integer({
  description: prIdDesc,
  minimum: 1,
});

export const ogSchema = Type.Union([
  Type.Object(
    { action: Type.Literal("auth_status"), project: Type.String({ description: projectDesc }) },
    { additionalProperties: false },
  ),
  Type.Object(
    { action: Type.Literal("pull"), project: Type.String({ description: projectDesc }) },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("push"),
      project: Type.String({ description: projectDesc }),
      force: Type.Optional(
        Type.Boolean({
          description: "Use force-with-lease; rejected on the default branch",
          default: false,
        }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("clone"),
      project: Type.Optional(Type.String({ description: projectDesc })),
      url: Type.Optional(
        Type.String({ description: "HTTP(S) repository URL with exactly owner/repo" }),
      ),
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
  Type.Object(
    {
      action: Type.Literal("pr_create"),
      project: Type.String({ description: projectDesc }),
      title: Type.String({ description: "Non-blank pull request title" }),
      body: Type.Optional(
        Type.String({ description: "Optional pull request body (may be multiline)" }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("pr_find"),
      project: Type.String({ description: projectDesc }),
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
      action: Type.Literal("pr_get"),
      project: Type.String({ description: projectDesc }),
      pr_id: Type.Optional(positivePrId),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("pr_checks"),
      project: Type.String({ description: projectDesc }),
      pr_id: Type.Optional(positivePrId),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("pr_modify"),
      project: Type.String({ description: projectDesc }),
      pr_id: Type.Optional(positivePrId),
      title: Type.Optional(Type.String({ description: "Replacement pull request title" })),
      body: Type.Optional(
        Type.String({ description: "Replacement pull request body; an empty string clears it" }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("pr_comment"),
      project: Type.String({ description: projectDesc }),
      pr_id: Type.Optional(positivePrId),
      body: Type.String({ description: "Non-blank pull request comment body (may be multiline)" }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: Type.Literal("pr_log"),
      project: Type.String({ description: projectDesc }),
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
      action: Type.Literal("pr_failures"),
      project: Type.String({ description: projectDesc }),
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
]);

export type OgInput = Static<typeof ogSchema>;

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

const OG_PROMPT_GUIDELINES = [
  "Use og with a registered project alias; every Git and forge mutation runs through the guarded daemon, so never pass paths, tokens, or credential environment names to og.",
  "Use og action auth_status to check forge authentication before retrying failed push or PR operations.",
  "Use og action clone with either a registered project or an HTTP(S) URL; project clones never accept alias or reference.",
  "Use og action pr_get without pr_id to inspect the registered checkout's current-branch pull request, or with a positive pr_id for a branch-free remote operation.",
  "Use og action pr_log and pr_failures to inspect CI logs with an optional tail between 0 and 1000 lines.",
  "Use og action pr_modify with at least one of title or body; an empty body explicitly clears it. Use og action pr_comment with a non-blank body.",
  "Use og action push with force only for force-with-lease; the daemon always rejects force on the default branch.",
];

type Action = OgInput;

function isAction<T extends Action["action"]>(
  input: OgInput,
  action: T,
): input is Extract<OgInput, { action: T }> {
  return input.action === action;
}

export function ogTool() {
  return {
    name: "og",
    label: "og",
    description:
      "Guarded repository and forge operations for registered projects: clone, push, pull, PR lifecycle, " +
      "comments, and CI status. All calls go through the og daemon, which owns credentials and policy.",
    promptSnippet: "Run guarded Git and forge operations through the og daemon",
    promptGuidelines: OG_PROMPT_GUIDELINES,
    parameters: ogSchema,
    async execute(
      _toolCallId: string,
      params: OgInput,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      _ctx: unknown,
    ): Promise<{ content: { type: "text"; text: string }[]; details: unknown }> {
      validateAction(params);
      const binary = resolveBinaryPath("og", { require });
      const { args, stdin } = buildArgs(params);
      const result = await runCli(binary, { args, stdin, signal });
      if (result.exitCode !== 0) {
        throw cliError(result.stderr, result.exitCode);
      }
      return render(params, result.stdout);
    },
  };
}

function validateAction(input: OgInput): void {
  if (isAction(input, "clone")) {
    const hasProject = !!input.project?.trim();
    const hasUrl = !!input.url?.trim();
    if (hasProject === hasUrl) {
      throw new Error("exactly one of project and url is required for clone");
    }
    if (hasProject && (input.alias || input.reference)) {
      throw new Error("project clone does not accept alias or reference");
    }
    if (input.reference && input.alias) {
      throw new Error("reference clone does not accept alias");
    }
    return;
  }
  if (isAction(input, "pr_modify")) {
    if (input.title === undefined && input.body === undefined) {
      throw new Error("pr_modify requires at least one of title or body");
    }
    if (input.title !== undefined && input.title.trim() === "") {
      throw new Error("PR title must not be blank");
    }
    return;
  }
  if (isAction(input, "pr_create") && input.title.trim() === "") {
    throw new Error("PR title must not be blank");
  }
  if (isAction(input, "pr_comment") && input.body.trim() === "") {
    throw new Error("comment body must not be blank");
  }
}

function buildArgs(input: OgInput): { args: string[]; stdin?: string } {
  const project = (
    "project" in input && input.project ? ["--project", input.project] : []
  ) as string[];
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
    const args = ["clone"];
    if (input.project) {
      args.push(input.project);
    } else {
      args.push(input.url!);
    }
    if (input.alias) {
      args.push("--alias", input.alias);
    }
    if (input.reference) {
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

function prIdArgs(input: { pr_id?: number }): string[] {
  return input.pr_id !== undefined ? ["--pr-id", String(input.pr_id)] : [];
}

function render(
  input: OgInput,
  stdout: string,
): { content: { type: "text"; text: string }[]; details: unknown } {
  if (isAction(input, "auth_status")) {
    const data = parseSingleJsonDoc<{ project?: string; auth: AuthStatus }>(stdout);
    const a = data.auth;
    const text =
      `${a.ready ? "Authenticated" : "Not authenticated"}: ${a.provider} ${a.host}/${a.owner}/${a.repo}` +
      (a.ready ? "" : ` (auth mode: ${a.auth_mode})`);
    return { content: [{ type: "text", text }], details: data };
  }
  if (isAction(input, "push") || isAction(input, "pull")) {
    const data = parseSingleJsonDoc<{ project?: string; message: string }>(stdout);
    return { content: [{ type: "text", text: data.message }], details: data };
  }
  if (isAction(input, "clone")) {
    const data = parseSingleJsonDoc<{ clone: CloneResult }>(stdout);
    const c = data.clone;
    const text = c.registered ? `Cloned ${c.alias} to ${c.path}` : `Cloned reference to ${c.path}`;
    return { content: [{ type: "text", text }], details: data };
  }
  if (isAction(input, "pr_comment")) {
    const data = parseSingleJsonDoc<{ project?: string; comment: Comment }>(stdout);
    return {
      content: [
        { type: "text", text: `Commented on PR #${data.comment.pr_id}: ${data.comment.url}` },
      ],
      details: data,
    };
  }
  if (isAction(input, "pr_log") || isAction(input, "pr_failures") || isAction(input, "pr_checks")) {
    const data = parseSingleJsonDoc<{ project?: string; pr: PullRequest; lines: string[] }>(stdout);
    const header = `PR #${data.pr.index} ${data.pr.title} [${data.pr.state}]`;
    const text = data.lines.length ? `${header}\n` + data.lines.join("\n") : header;
    return { content: [{ type: "text", text }], details: data };
  }
  const data = parseSingleJsonDoc<{ project?: string; pr: PullRequest }>(stdout);
  const pr = data.pr;
  const text = `PR #${pr.index} ${pr.title} [${pr.state}]${pr.url ? `\n${pr.url}` : ""}`;
  return { content: [{ type: "text", text }], details: data };
}

export function registerOgTool(pi: {
  registerTool(definition: ReturnType<typeof ogTool>): void;
}): void {
  pi.registerTool(ogTool());
}
