export interface PublishPlanEntry {
  kind: "native" | "main";
  dir: string;
  name: string;
  path: string;
}

export function packagePublishPlan(workspace?: string): PublishPlanEntry[];
export function assertNativePackagesFirst(plan: readonly PublishPlanEntry[]): void;
export function publishReleasePackages(plan?: readonly PublishPlanEntry[]): void;
