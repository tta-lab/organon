export const SETTINGS_NAMESPACE = "organon-web" as const;
export const SEARCH_PROVIDER_ID = "organon-web-search" as const;

export const PROVIDERS = ["exa", "brave", "duckduckgo"] as const;
export type SearchProviderName = (typeof PROVIDERS)[number];

/** DSH-owned references; these names never identify a value in settings. */
export const EXA_CREDENTIAL_REF = "ORGANON_DSH_WEB_EXA_API_KEY" as const;
export const BRAVE_CREDENTIAL_REF = "ORGANON_DSH_WEB_BRAVE_API_KEY" as const;
export const CREDENTIAL_REFS = [EXA_CREDENTIAL_REF, BRAVE_CREDENTIAL_REF] as const;

export interface OrganonSettings {
  provider: SearchProviderName;
}

export function isSearchProvider(value: unknown): value is SearchProviderName {
  return typeof value === "string" && (PROVIDERS as readonly string[]).includes(value);
}
