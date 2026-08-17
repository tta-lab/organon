export interface PublishPlanEntry {
  kind: "native" | "main";
  dir: string;
  name: string;
  version: string;
  path: string;
}

export interface PublishOptions {
  npmCommand?: string;
  registry?: string;
  env?: NodeJS.ProcessEnv;
}

export interface PublishResult {
  version: string;
  distTag: string;
  published: string[];
  skipped: string[];
}

export const NPM_REGISTRY: string;
export function packagePublishPlan(workspace?: string): PublishPlanEntry[];
export function assertNativePackagesFirst(plan: readonly PublishPlanEntry[]): void;
export function distTagForVersion(version: string): "beta" | "latest";
export function publishReleasePackages(
  plan?: readonly PublishPlanEntry[],
  options?: PublishOptions,
): PublishResult;
