import { createRequire } from "node:module";

import { Type, type Static } from "typebox";

import {
  cliError,
  modelTextResult,
  parseSingleJsonDoc,
  resolveBinaryPath,
  runCli,
} from "@tta-lab/pi-shared";

const require = createRequire(import.meta.url);

export const projectListSchema = Type.Object(
  {
    include_archived: Type.Optional(
      Type.Boolean({ description: "Include archived projects; defaults to false" }),
    ),
  },
  { additionalProperties: false },
);

export const projectFindSchema = Type.Object(
  {
    query: Type.String({
      description: "Non-blank natural-language query for active projects and references",
      minLength: 1,
    }),
    limit: Type.Optional(
      Type.Integer({
        description: "Maximum results; defaults to 8 and is capped at 32 by the core finder",
        minimum: 1,
        default: 8,
      }),
    ),
  },
  { additionalProperties: false },
);

export const projectGetSchema = Type.Object(
  {
    project: Type.String({
      description:
        "Project reference: canonical alias, checkout basename, or remote repository basename",
      minLength: 1,
    }),
  },
  { additionalProperties: false },
);

export type ProjectListInput = Static<typeof projectListSchema>;
export type ProjectFindInput = Static<typeof projectFindSchema>;
export type ProjectGetInput = Static<typeof projectGetSchema>;

export interface ProjectListResult {
  projects: ProjectRecord[];
}
export interface ProjectGetResult {
  project: ProjectRecord;
}
export interface ProjectRecord {
  alias: string;
  name?: string;
  path: string;
  remote?: string;
  archived: boolean;
  reference?: boolean;
}

function requireNonBlankString(input: unknown, field: string): string {
  if (
    typeof input !== "object" ||
    input === null ||
    typeof (input as Record<string, unknown>)[field] !== "string" ||
    !/\S/.test((input as Record<string, unknown>)[field] as string)
  ) {
    throw new Error(`${field} must be a non-blank string`);
  }
  return (input as Record<string, unknown>)[field] as string;
}

function renderProject(p: ProjectRecord): string {
  const label = p.name && p.name !== "" ? p.name : p.alias;
  const referenceSuffix = p.reference ? " [reference]" : "";
  return `${p.alias}${referenceSuffix}: ${label} (${p.path})`;
}

function makeProjectTool(options: {
  name: string;
  label: string;
  description: string;
  promptSnippet: string;
  promptGuidelines: string[];
  parameters: object;
  args(params: unknown): string[];
  render(
    params: unknown,
    stdout: string,
  ): Promise<{ content: { type: "text"; text: string }[]; details: unknown }>;
}) {
  const binary = resolveBinaryPath("project", { require });
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
    ) {
      const result = await runCli(binary, { args: options.args(params), signal });
      if (result.exitCode !== 0) throw await cliError(result.stderr, result.exitCode);
      return options.render(params, result.stdout);
    },
  };
}

export function projectListTool() {
  return makeProjectTool({
    name: "project_list",
    label: "Project list",
    description:
      "List registered Organon projects, including archived projects when requested. Text output is limited to 2,000 lines or 50KB; truncated output is saved to a temporary file.",
    promptSnippet: "List registered projects with project_list",
    promptGuidelines: [
      "Use project_list to enumerate registered Organon projects and their canonical paths.",
      "Set include_archived when you need archived projects or recovery after an ambiguous reference.",
    ],
    parameters: projectListSchema,
    args(params) {
      if (typeof params !== "object" || params === null || Array.isArray(params)) {
        throw new Error("project_list input must be an object");
      }
      const input = params as ProjectListInput;
      if (input.include_archived !== undefined && typeof input.include_archived !== "boolean") {
        throw new Error("include_archived must be a boolean");
      }
      return ["list", "--json", ...(input.include_archived ? ["--include-archived"] : [])];
    },
    async render(_params, stdout) {
      const data = parseSingleJsonDoc<ProjectListResult>(stdout);
      const lines = data.projects.map(
        (p) => `- ${renderProject(p)}${p.archived ? " [archived]" : ""}`,
      );
      const raw =
        lines.length === 0 ? "No projects found." : "Available projects:\n" + lines.join("\n");
      return modelTextResult(data, raw, {
        hint: "Use project_get with an exact project reference to inspect one project.",
      });
    },
  });
}

export function projectFindTool() {
  return makeProjectTool({
    name: "project_find",
    label: "Project find",
    description:
      "Find active Organon projects and locally cloned references by alias, display name, checkout name, or repository name. Registered projects take precedence over same-named references.",
    promptSnippet: "Find active projects and references with project_find",
    promptGuidelines: [
      "Use project_find to discover active projects and local references by alias, display name, checkout name, or repository name.",
      "Use a registered result's canonical alias with project_get; reference results expose their local path directly.",
    ],
    parameters: projectFindSchema,
    args(params) {
      const input = params as ProjectFindInput;
      const query = requireNonBlankString(input, "query");
      if (input.limit !== undefined && (!Number.isInteger(input.limit) || input.limit < 1)) {
        throw new Error("limit must be a positive integer");
      }
      return [
        "find",
        query,
        ...(input.limit !== undefined ? ["--limit", String(input.limit)] : []),
        "--json",
      ];
    },
    async render(_params, stdout) {
      const data = parseSingleJsonDoc<ProjectListResult>(stdout);
      const lines = data.projects.map((p) => `- ${renderProject(p)}`);
      const raw =
        lines.length === 0
          ? "No active projects or references found."
          : "Matching projects and references:\n" + lines.join("\n");
      return modelTextResult(data, raw, {
        hint: "Use project_get for registered results; reference results expose their local path directly.",
      });
    },
  });
}

export function projectGetTool() {
  return makeProjectTool({
    name: "project_get",
    label: "Project get",
    description:
      "Get one registered Organon project by an exact canonical alias, checkout basename, or remote repository basename.",
    promptSnippet: "Get one registered project with project_get",
    promptGuidelines: [
      "Use project_get when you have a canonical alias, checkout basename, or remote repository basename; successful results return the canonical alias.",
      "Use project_find or project_list if you need to recover another project reference.",
    ],
    parameters: projectGetSchema,
    args(params) {
      return ["get", requireNonBlankString(params, "project"), "--json"];
    },
    async render(_params, stdout) {
      const data = parseSingleJsonDoc<ProjectGetResult>(stdout);
      const p = data.project;
      const text =
        p.name && p.name !== "" ? `${p.alias}: ${p.name} (${p.path})` : `${p.alias}: ${p.path}`;
      return modelTextResult(data, text, {
        hint: "Use project_find or project_list if you need to recover another project reference.",
      });
    },
  });
}

export function registerProjectTools(pi: { registerTool(definition: any): void }): void {
  pi.registerTool(projectListTool());
  pi.registerTool(projectFindTool());
  pi.registerTool(projectGetTool());
}
