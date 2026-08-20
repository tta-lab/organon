import { createElement, useEffect, useState } from "react";
import type { ConnectionHandle, IApiClient } from "@deepseek-ai/dsh-client-connection/client";
import type {
  ClientContext,
  SettingsScope,
  SettingsScopeSpec,
} from "@deepseek-ai/dsh-client-runtime/client";
import type {} from "@deepseek-ai/dsh-api-remotes/client";
import type {} from "@deepseek-ai/dsh-client-ui-settings/client";
import type {} from "@deepseek-ai/dsh-client-ui-settings-plugins/client";
import type {} from "@deepseek-ai/dsh-client-ui-slots";

import {
  BRAVE_CREDENTIAL_REF,
  CREDENTIAL_REFS,
  EXA_CREDENTIAL_REF,
  PROVIDERS,
  SETTINGS_NAMESPACE,
  isSearchProvider,
  type OrganonSettings,
  type SearchProviderName,
} from "./contract.js";

export const inject = ["connection", "remote", "settingsScope", "slots"] as const;

type CredentialApi = Pick<IApiClient, "credentials">;
type RemoteApi = {
  $on(event: "credentials/updated", listener: (ref: string) => void): () => void;
};
export interface CredentialStatus {
  configured: boolean;
  source?: string;
  writable: boolean;
}

type ClientSettings = Partial<OrganonSettings>;
export type OrganonSettingsScope = SettingsScope<ClientSettings>;
export type OrganonSettingsScopeSpec = SettingsScopeSpec<ClientSettings>;

export function decodeSettings(value: unknown): ClientSettings | undefined {
  if (typeof value !== "object" || value === null) return undefined;
  const provider = (value as { provider?: unknown }).provider;
  return isSearchProvider(provider) ? { provider } : {};
}

export async function persistProviderSelection(
  scope: Pick<SettingsScope<ClientSettings>, "set">,
  provider: SearchProviderName,
): Promise<void> {
  await scope.set("provider", provider);
}

export async function describeCredentialStatus(
  api: CredentialApi,
  refs: readonly string[],
): Promise<Record<string, CredentialStatus>> {
  const response = await api.credentials.describe({ refs: [...refs] });
  if (!response.result.ok) throw new Error("credential status unavailable");
  const views = response.result.value.credentials;
  return Object.fromEntries(
    refs.map((ref) => {
      const view = views[ref];
      return [
        ref,
        {
          configured: view?.configured === true,
          ...(typeof view?.source === "string" && view.source.length > 0
            ? { source: view.source }
            : {}),
          writable: view?.writable === true,
        },
      ];
    }),
  );
}

export async function writeCredential(
  api: CredentialApi,
  ref: string,
  draft: string,
): Promise<boolean> {
  if (draft.trim() === "") return false;
  const response = await api.credentials.set({ ref, value: draft });
  if (!response.result.ok) throw new Error("credential write rejected");
  return true;
}

export async function removeCredential(api: CredentialApi, ref: string): Promise<void> {
  const response = await api.credentials.unset({ ref });
  if (!response.result.ok) throw new Error("credential removal rejected");
}

function SettingsCard({
  scope,
  api,
  remote,
}: {
  scope: OrganonSettingsScope;
  api: CredentialApi;
  remote: RemoteApi;
}) {
  const [snapshot, setSnapshot] = useState(scope.getSnapshot());
  const [status, setStatus] = useState<Record<string, CredentialStatus>>({});
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [credentialRevision, setCredentialRevision] = useState(0);
  const [error, setError] = useState<string>();

  useEffect(() => scope.subscribe(() => setSnapshot(scope.getSnapshot())), [scope]);
  useEffect(
    () =>
      remote.$on("credentials/updated", (ref) => {
        if ((CREDENTIAL_REFS as readonly string[]).includes(ref)) {
          setCredentialRevision((revision) => revision + 1);
        }
      }),
    [remote],
  );
  useEffect(() => {
    let active = true;
    void describeCredentialStatus(api, CREDENTIAL_REFS)
      .then((next) => {
        if (active) setStatus(next);
      })
      .catch(() => {
        if (active) setError("Credential status is unavailable.");
      });
    return () => {
      active = false;
    };
  }, [api, credentialRevision]);

  const provider = isSearchProvider(snapshot.value?.provider)
    ? snapshot.value.provider
    : "duckduckgo";
  const saveProvider = async (next: SearchProviderName) => {
    setError(undefined);
    try {
      await persistProviderSelection(scope, next);
    } catch {
      setError("Provider selection could not be saved.");
    }
  };
  const saveCredential = async (ref: string) => {
    setError(undefined);
    try {
      const wrote = await writeCredential(api, ref, drafts[ref] ?? "");
      if (wrote) {
        setDrafts((current) => ({ ...current, [ref]: "" }));
        setCredentialRevision((revision) => revision + 1);
      }
    } catch {
      setError("Credential could not be saved.");
    }
  };
  const clearCredential = async (ref: string) => {
    setError(undefined);
    try {
      await removeCredential(api, ref);
      setCredentialRevision((revision) => revision + 1);
    } catch {
      setError("Credential could not be removed.");
    }
  };

  return createElement(
    "section",
    { "aria-labelledby": "organon-web-settings-title" },
    createElement("h2", { id: "organon-web-settings-title" }, "Organon Web"),
    createElement(
      "p",
      null,
      "Choose the search provider explicitly. DuckDuckGo does not require a key.",
    ),
    createElement(
      "label",
      { htmlFor: "organon-web-provider" },
      "Search provider",
      createElement(
        "select",
        {
          id: "organon-web-provider",
          value: provider,
          onChange: (event: { target: { value: string } }) => {
            if (isSearchProvider(event.target.value)) void saveProvider(event.target.value);
          },
        },
        ...PROVIDERS.map((value) => createElement("option", { key: value, value }, value)),
      ),
    ),
    ...CREDENTIAL_REFS.map((ref) => {
      const label =
        ref === EXA_CREDENTIAL_REF
          ? "Exa API key"
          : ref === BRAVE_CREDENTIAL_REF
            ? "Brave API key"
            : "Context7 API key";
      const current = status[ref] ?? { configured: false, writable: false };
      return createElement(
        "fieldset",
        { key: ref },
        createElement("legend", null, label),
        createElement(
          "label",
          { htmlFor: `organon-web-${ref}` },
          "Write a new key",
          createElement("input", {
            id: `organon-web-${ref}`,
            type: "password",
            autoComplete: "new-password",
            value: drafts[ref] ?? "",
            disabled: !current.writable,
            onChange: (event: { target: { value: string } }) =>
              setDrafts((draft) => ({ ...draft, [ref]: event.target.value })),
          }),
        ),
        createElement(
          "p",
          { role: "status" },
          `Status: ${current.configured ? "configured" : "not configured"}; source: ${current.source ?? "none"}; writable: ${current.writable ? "yes" : "no"}.`,
        ),
        createElement(
          "button",
          {
            type: "button",
            disabled: !current.writable || (drafts[ref] ?? "").trim() === "",
            onClick: () => void saveCredential(ref),
          },
          "Save key",
        ),
        createElement(
          "button",
          {
            type: "button",
            disabled: !current.writable || !current.configured,
            onClick: () => void clearCredential(ref),
          },
          "Remove key",
        ),
      );
    }),
    error ? createElement("p", { role: "alert" }, error) : null,
  );
}

export function apply(ctx: ClientContext): void {
  const connection = ctx.get("connection") as ConnectionHandle;
  const remote = ctx.get("remote") as RemoteApi;
  const scope = ctx.settingsScope.bind<ClientSettings>({
    namespace: SETTINGS_NAMESPACE,
    decode: decodeSettings,
  });

  ctx.slots.inject("settings.plugin.item", () =>
    ctx.slots.register(
      {
        name: "settings.plugin.item",
        key: SETTINGS_NAMESPACE,
        inject: () => ({}),
      } as never,
      (() => createElement(SettingsCard, { scope, api: connection.api, remote })) as never,
    ),
  );
}
