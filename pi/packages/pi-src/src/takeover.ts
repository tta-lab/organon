import type { ExtensionAPI, ToolInfo } from "@earendil-works/pi-coding-agent";

/** Built-in tools the src extension may displace while active. */
const DISPLACED_BUILTINS = ["read", "edit"] as const;

type DisplacedName = (typeof DISPLACED_BUILTINS)[number];

interface TakeoverAPI {
  getActiveTools(): string[];
  getAllTools(): ToolInfo[];
  setActiveTools(names: string[]): void;
}

/**
 * Provenance-based active-tool takeover for pi-src.
 *
 * At session_start the extension removes only ACTIVE tools whose provenance is
 * the Pi builtin and whose names are read or edit, retains every unrelated
 * active tool, activates the src tool, and remembers exactly which built-ins
 * this instance displaced. At session_shutdown it re-adds only that remembered
 * subset, preserving every other current active-tool choice; a replacement
 * extension instance applies the policy again after reload or replacement.
 */
export function applyReadTakeover(pi: TakeoverAPI): string[] {
  const all = pi.getAllTools();
  const active = new Set(pi.getActiveTools());
  const displaced: DisplacedName[] = [];
  for (const name of DISPLACED_BUILTINS) {
    const tool = all.find((t) => t.name === name && t.sourceInfo?.source === "builtin");
    if (tool && active.has(name)) {
      displaced.push(name);
    }
  }
  const removed = new Set(displaced);
  // The src tool is always activated: removed displaced built-ins are dropped,
  // every unrelated active tool is retained, and src joins the set.
  const next = [
    ...pi.getActiveTools().filter((name) => !removed.has(name as DisplacedName)),
    "src",
  ];
  pi.setActiveTools([...new Set(next)]);
  return displaced;
}

/**
 * Restores only the built-ins this extension instance displaced, without
 * disturbing other active-tool choices made since session start.
 */
export function restoreReadTakeover(pi: TakeoverAPI, displaced: string[]): void {
  if (displaced.length === 0) {
    return;
  }
  const current = pi.getActiveTools();
  const restored = [...new Set([...current, ...displaced])];
  pi.setActiveTools(restored);
}

/** Convenience for the extension entry: hold displaced state across events. */
export function createTakeoverHandlers(pi: ExtensionAPI): {
  onSessionStart(): void;
  onSessionShutdown(): void;
} {
  let displaced: string[] = [];
  return {
    onSessionStart() {
      displaced = applyReadTakeover(pi);
    },
    onSessionShutdown() {
      restoreReadTakeover(pi, displaced);
      displaced = [];
    },
  };
}
