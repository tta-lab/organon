import { createRequire } from "node:module";

import type { Context } from "@deepseek-ai/cordis";
import { settingsNamespace } from "@deepseek-ai/dsh-settings";
import z from "@deepseek-ai/schemastery";
import { resolveBinaryPath } from "@tta-lab/pi-shared/binary";

import { PROVIDERS, SETTINGS_NAMESPACE, type OrganonSettings } from "./contract.js";
import { createOrganonSearchProvider } from "./provider.js";

export const name = "organon-dsh-web";
export const inject = ["web", "credentials", "settings"] as const;

export const SettingsSchema = z.object({
  provider: z.union(PROVIDERS).default("duckduckgo"),
});

export function apply(ctx: Context): void {
  const settings = ctx.settings.register(settingsNamespace(SETTINGS_NAMESPACE), SettingsSchema, {
    base: { provider: "duckduckgo" },
    applies: "live",
  });
  const binaryPath = resolveBinaryPath("web", { require: createRequire(import.meta.url) });
  const provider = createOrganonSearchProvider({
    binaryPath,
    getProvider: () => (settings.get() as OrganonSettings).provider,
    credentials: ctx.credentials,
  });
  ctx.web.registerSearchProvider(provider);
}
