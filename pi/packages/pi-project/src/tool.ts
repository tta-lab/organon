import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import { Type, type Static } from "typebox";

import {
  objectUnion,
  resolveBinaryPath,
  runCli,
  parseSingleJsonDoc,
  cliError,
  modelTextResult,
} from "@tta-lab/pi-shared";

const require = createRequire(import.meta.url);

export const projectSchema = objectUnion([
  Type.Object(
    {
      action: StringEnum(["list"] as const, { description: "Action to perform" }),
      include_archived: Type.Optional(
        Type.Boolean({ description: "Include archived projects; defaults to false" }),
      ),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["get"] as const, { description: "Action to perform" }),
      project: Type.String({
        description:
          "Project reference: canonical alias, checkout basename, or remote repository basename",
        minLength: 1,
      }),
    },
    { additionalProperties: false },
  ),
  Type.Object(
    {
      action: StringEnum(["find"] as const, { description: "Action to perform" }),
      query: Type.String({
        description: "Non-blank natural-language query for active projects",
        minLength: 1,
      }),
      limit: Type.Optional(
        Type.Integer({
          description:
            "Maximum active projects; defaults to 8 and is capped at 32 by the core finder",
          minimum: 1,
          default: 8,
        }),
      ),
    },
    { additionalProperties: false },
  ),
]);

export type ProjectInput = Static<typeof projectSchema>;

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
}

const LIST_PROMPT_GUIDELINES = [
  "Use project with action list to enumerate registered Organon projects and their canonical paths.",
  "Use project with action get when you have a canonical alias, checkout basename, or remote repository basename; successful results return the canonical alias.",
  "Use project with action find to discover active projects by alias, display name, checkout name, or repository name. Fuzzy results are advisory and never select a project.",
  "Use project with action list when you need archived projects or recovery after an ambiguous reference.",
];

function isList(input: ProjectInput): input is Extract<ProjectInput, { action: "list" }> {
  return input.action === "list";
}

function isFind(input: ProjectInput): input is Extract<ProjectInput, { action: "find" }> {
  return input.action === "find";
}

export function projectTool() {
  const binary = resolveBinaryPath("project", { require });
  return {
    name: "project",
    label: "Project",
    description:
      "List registered Organon projects, find active projects, or get one by an exact project reference. " +
      "References are canonical aliases, checkout basenames, or remote repository basenames; display names are discovery-only and fuzzy matches never select a project. " +
      "Text output is limited to 2,000 lines or 50KB; truncated output is saved to a temporary file.",
    promptSnippet: "List, find, or get registered projects by project reference",
    promptGuidelines: LIST_PROMPT_GUIDELINES,
    parameters: projectSchema,
    async execute(
      _toolCallId: string,
      params: ProjectInput,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      _ctx: unknown,
    ): Promise<{ content: { type: "text"; text: string }[]; details: unknown }> {
      const args = isList(params)
        ? ["list", "--json", ...(params.include_archived ? ["--include-archived"] : [])]
        : isFind(params)
          ? [
              "find",
              params.query,
              ...(params.limit !== undefined ? ["--limit", String(params.limit)] : []),
              "--json",
            ]
          : ["get", params.project, "--json"];
      const result = await runCli(binary, { args, signal });
      if (result.exitCode !== 0) {
        throw await cliError(result.stderr, result.exitCode);
      }
      if (isList(params)) {
        const data = parseSingleJsonDoc<ProjectListResult>(result.stdout);
        const lines = data.projects.map((p) => {
          const label = p.name && p.name !== "" ? p.name : p.alias;
          return `- ${p.alias}: ${label} (${p.path})${p.archived ? " [archived]" : ""}`;
        });
        const raw =
          lines.length === 0 ? "No projects found." : "Available projects:\n" + lines.join("\n");
        return modelTextResult(data, raw, {
          hint: "Use project get with an exact project reference to inspect one project.",
        });
      }
      if (isFind(params)) {
        const data = parseSingleJsonDoc<ProjectListResult>(result.stdout);
        const lines = data.projects.map((p) => {
          const label = p.name && p.name !== "" ? p.name : p.alias;
          return `- ${p.alias}: ${label} (${p.path})`;
        });
        const raw =
          lines.length === 0
            ? "No active projects found."
            : "Matching active projects:\n" + lines.join("\n");
        return modelTextResult(data, raw, {
          hint: "Use project get with a canonical alias from the results to target one project.",
        });
      }
      const data = parseSingleJsonDoc<ProjectGetResult>(result.stdout);
      const p = data.project;
      const text =
        p.name && p.name !== "" ? `${p.alias}: ${p.name} (${p.path})` : `${p.alias}: ${p.path}`;
      return modelTextResult(data, text, {
        hint: "Use project find or project list if you need to recover another project reference.",
      });
    },
  };
}

export function registerProjectTool(pi: {
  registerTool(definition: ReturnType<typeof projectTool>): void;
}): void {
  pi.registerTool(projectTool());
}
