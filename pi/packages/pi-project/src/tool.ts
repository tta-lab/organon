import { createRequire } from "node:module";

import { StringEnum } from "@earendil-works/pi-ai";
import { Type, type Static } from "typebox";

import {
  resolveBinaryPath,
  runCli,
  parseSingleJsonDoc,
  cliError,
  truncateForModel,
} from "@tta-lab/pi-shared";

const require = createRequire(import.meta.url);

export const projectSchema = Type.Union([
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
      alias: Type.String({
        description: "Exact registered single-layer project alias",
      }),
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
  "Use project with action get when you need one registered project by its exact alias; the alias must match the returned alias exactly.",
];

function isList(input: ProjectInput): input is Extract<ProjectInput, { action: "list" }> {
  return input.action === "list";
}

export function projectTool() {
  return {
    name: "project",
    label: "Project",
    description:
      "List registered Organon projects or get one registered project by exact alias. " +
      "The alias is the single-layer registered alias from the projects catalog, never an org/repo string. " +
      "Text output is limited to 2,000 lines or 50KB; truncated output is saved to a temporary file.",
    promptSnippet: "List registered projects or get one by exact alias",
    promptGuidelines: LIST_PROMPT_GUIDELINES,
    parameters: projectSchema,
    async execute(
      _toolCallId: string,
      params: ProjectInput,
      signal: AbortSignal | undefined,
      _onUpdate: undefined,
      _ctx: unknown,
    ): Promise<{ content: { type: "text"; text: string }[]; details: unknown }> {
      const binary = resolveBinaryPath("project", { require });
      const args = isList(params)
        ? ["list", "--json", ...(params.include_archived ? ["--include-archived"] : [])]
        : ["get", params.alias, "--json"];
      const result = await runCli(binary, { args, signal });
      if (result.exitCode !== 0) {
        throw cliError(result.stderr, result.exitCode);
      }
      if (isList(params)) {
        const data = parseSingleJsonDoc<ProjectListResult>(result.stdout);
        const lines = data.projects.map((p) => {
          const label = p.name && p.name !== "" ? p.name : p.alias;
          return `- ${p.alias}: ${label} (${p.path})${p.archived ? " [archived]" : ""}`;
        });
        const raw =
          lines.length === 0 ? "No projects found." : "Available projects:\n" + lines.join("\n");
        const model = await truncateForModel(raw, {
          hint: "Use project get with an exact alias to inspect one project.",
        });
        return {
          content: [{ type: "text", text: model.text }],
          details: model.truncation
            ? { ...data, truncation: model.truncation, fullOutputPath: model.fullOutputPath }
            : data,
        };
      }
      const data = parseSingleJsonDoc<ProjectGetResult>(result.stdout);
      const p = data.project;
      const text =
        p.name && p.name !== "" ? `${p.alias}: ${p.name} (${p.path})` : `${p.alias}: ${p.path}`;
      const model = await truncateForModel(text, {
        hint: "Use project list or get with an exact alias to narrow the result.",
      });
      return {
        content: [{ type: "text", text: model.text }],
        details: model.truncation
          ? { ...data, truncation: model.truncation, fullOutputPath: model.fullOutputPath }
          : data,
      };
    },
  };
}

export function registerProjectTool(pi: {
  registerTool(definition: ReturnType<typeof projectTool>): void;
}): void {
  pi.registerTool(projectTool());
}
